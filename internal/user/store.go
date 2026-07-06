// Package user stores and looks up user accounts
package user

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/loqui-chat/loqui-backend/internal/auth"
	"github.com/loqui-chat/loqui-backend/internal/snowflake"
)

type User struct {
	ID            int64
	Username      string
	Discriminator string
	Email         *string
	PasswordHash  string
	CreatedAt     time.Time
}

var (
	ErrEmailTaken      = errors.New("user: email already registered")
	ErrNotFound        = errors.New("user: not found")
	ErrNoDiscriminator = errors.New("user: no free discriminator for this username")
)

const maxDiscriminatorTries = 5

type Store struct {
	pool *pgxpool.Pool
	gen  *snowflake.Generator
}

func NewStore(pool *pgxpool.Pool, gen *snowflake.Generator) *Store {
	return &Store{pool: pool, gen: gen}
}

// Create inserts a new user, retrying on discriminator collisions
func (s *Store) Create(ctx context.Context, username string, email *string, passwordHash string) (*User, error) {
	for range maxDiscriminatorTries {
		disc, err := auth.NewDiscriminator()
		if err != nil {
			return nil, err
		}
		id, err := s.gen.Next()
		if err != nil {
			return nil, err
		}

		u := &User{
			ID:            id,
			Username:      username,
			Discriminator: disc,
			Email:         email,
			PasswordHash:  passwordHash,
		}
		err = s.pool.QueryRow(
			ctx, `
			insert into users (id, username, discriminator, email, password_hash)
			values ($1, $2, $3, $4, $5)
			returning created_at`,
			u.ID, u.Username, u.Discriminator, u.Email, u.PasswordHash,
		).Scan(&u.CreatedAt)
		if err == nil {
			return u, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "user_identity_key":
				continue // discriminator taken for this username, retry
			case "users_email_key":
				return nil, ErrEmailTaken
			}
		}
		return nil, err
	}
	return nil, ErrNoDiscriminator
}

// GetByID looks up a user by id
func (s *Store) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.scanOne(ctx, `
			select id, username, discriminator, email, password_hash, created_at
			from users where id = $1`, id)
}

// GetByIdentity looks up a user by username (case insensitive) and discriminator
func (s *Store) GetByIdentity(ctx context.Context, username, discriminator string) (*User, error) {
	return s.scanOne(ctx, `
		select id, username, discriminator, email, password_hash, created_at
		from users where lower(username) = lower($1) and discriminator = $2`,
		username, discriminator)
}

// GetByEmail looks up a user by email (case insensitive)
func (s *Store) GetByEmail(ctx context.Context, email string) (*User, error) {
	return s.scanOne(ctx, `
		select id, username, discriminator, email, password_hash, created_at
		from users where lower(email) = lower($1)`, email)
}

func (s *Store) scanOne(ctx context.Context, query string, args ...any) (*User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&u.ID, &u.Username, &u.Discriminator, &u.Email, &u.PasswordHash, &u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
