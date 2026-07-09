// Package session manages refresh tokens
// tokens are hashed opaque secrets, rotated on use > reuse means leak
// access tokens remain stateless JWTS
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/loqui-chat/loqui-backend/internal/snowflake"
)

var (
	ErrInvalidRefresh = errors.New("session: invalid refresh token")
	ErrReuseDetected  = errors.New("session: refresh token reuse detected")
)

const secretBytes = 32

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Store struct {
	pool       *pgxpool.Pool
	gen        *snowflake.Generator
	refreshTTL time.Duration
}

func NewStore(pool *pgxpool.Pool, gen *snowflake.Generator, refreshTTL time.Duration) *Store {
	return &Store{pool: pool, gen: gen, refreshTTL: refreshTTL}
}

// Issued is a freshly minted opaque refresh token and its expiry
type Issued struct {
	Token     string
	ExpiresAt time.Time
}

// Issue starts a new session (token family) and returns its first token
func (s *Store) Issue(ctx context.Context, userID int64, userAgent string) (*Issued, error) {
	id, err := s.gen.Next()
	if err != nil {
		return nil, err
	}
	return s.insert(ctx, s.pool, id, userID, id, ptrOrNil(userAgent))
}

// Rotate swaps a valid token for a fresh one in same family, returning it
// with the owing user id. replaying a spent token revokes whole family
func (s *Store) Rotate(ctx context.Context, token string) (*Issued, int64, error) {
	id, secret, ok := decodeToken(token)
	if !ok {
		return nil, 0, ErrInvalidRefresh
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		userID    int64
		familyID  int64
		hash      string
		userAgent *string
		expiresAt time.Time
		rotatedAt *time.Time
		revokedAt *time.Time
	)
	err = tx.QueryRow(
		ctx, `
		select user_id, family_id, token_hash, user_agent, expires_at, rotated_at, revoked_at
		from refresh_tokens where id = $1 for update`, id,
	).Scan(&userID, &familyID, &hash, &userAgent, &expiresAt, &rotatedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrInvalidRefresh
	}
	if err != nil {
		return nil, 0, err
	}

	// wrong secret for real id: reject without touching row
	if !secretMatches(secret, hash) {
		return nil, 0, ErrInvalidRefresh
	}
	if revokedAt != nil {
		return nil, 0, ErrInvalidRefresh
	}
	if rotatedAt != nil {
		// spent token replayed: kill whole family
		if _, err := tx.Exec(ctx, `
			update refresh_tokens set revoked_at = now()
			where family_id = $1 and revoked_at is null`, familyID); err != nil {
			return nil, 0, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, 0, err
		}
		return nil, 0, ErrReuseDetected
	}
	if !expiresAt.After(time.Now()) {
		return nil, 0, ErrInvalidRefresh
	}

	if _, err := tx.Exec(ctx, `
		update refresh_tokens set rotated_at = now(), last_used_at = now()
		where id = $1`, id); err != nil {
		return nil, 0, err
	}

	newID, err := s.gen.Next()
	if err != nil {
		return nil, 0, err
	}
	issued, err := s.insert(ctx, tx, newID, userID, familyID, userAgent)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return issued, userID, nil
}

// Revoke ends tokens session. the secret is checked so an id alone cant
// kill someone elses session, unknown or bad tokens are a silent no-op
func (s *Store) Revoke(ctx context.Context, token string) error {
	id, secret, ok := decodeToken(token)
	if !ok {
		return nil
	}
	var (
		familyID int64
		hash     string
	)
	err := s.pool.QueryRow(
		ctx, `
		select family_id, token_hash from refresh_tokens where id = $1`, id,
	).Scan(&familyID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !secretMatches(secret, hash) {
		return nil
	}
	_, err = s.pool.Exec(ctx, `
		update refresh_tokens set revoked_at = now()
		where family_id = $1 and revoked_at is null`, familyID)
	return err
}

// RunGC deletes expired and revoked rows on an interval until ctx is done
// live spent tokens stay so reuse detection still fires within their lifetime
func (s *Store) RunGC(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.pool.Exec(ctx, `
				delete from refresh_tokens
				where expires_at < now() or revoked_at is not null`)
		}
	}
}

// insert write one token row and returns its opaque form (q is a pool or tx)
func (s *Store) insert(ctx context.Context, q querier, id, userID, familyID int64, userAgent *string) (*Issued, error) {
	secret, hash, err := newSecret()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(s.refreshTTL)
	if _, err := q.Exec(
		ctx, `
		insert into refresh_tokens (id, user_id, family_id, token_hash, user_agent, expires_at)
		values ($1, $2, $3, $4, $5, $6)`,
		id, userID, familyID, hash, userAgent, expires,
	); err != nil {
		return nil, err
	}
	return &Issued{Token: encodeToken(id, secret), ExpiresAt: expires}, nil
}

// token wire form is "<id>.<secret>": id is lookup key, secret stays private
func encodeToken(id int64, secret string) string {
	return strconv.FormatInt(id, 10) + "." + secret
}

func decodeToken(token string) (id int64, secret string, ok bool) {
	idStr, secret, found := strings.Cut(token, ".")
	if !found || idStr == "" || secret == "" {
		return 0, "", false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return id, secret, true
}

func newSecret() (secret, hash string, err error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	secret = base64.RawURLEncoding.EncodeToString(b)
	return secret, hashSecret(secret), nil
}

// hashSecret is plain sha256: the secret is already full-entropy random
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func secretMatches(secret, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(hash)) == 1
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
