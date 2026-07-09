package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokeType string

const (
	AccessToken  TokeType = "access"
	RefreshToken TokeType = "refresh"
)

var ErrInvalidToken = errors.New("auth: invalid token")

// Claims is the JWT payload: a token plus standaed registered claims
type Claims struct {
	Type TokeType `json:"typ"`
	jwt.RegisteredClaims
}

// Issuer signs and verifies EdDSA (ed25519) JWTs
type Issuer struct {
	priv       ed25519.PrivateKey
	pub        ed25519.PublicKey
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewIssuer(priv ed25519.PrivateKey, accessTTL, refreshTTL time.Duration) *Issuer {
	return &Issuer{
		priv:       priv,
		pub:        priv.Public().(ed25519.PublicKey),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// Issue returns a signed token of given type for a user id
func (i *Issuer) Issue(userID int64, typ TokeType) (string, error) {
	ttl := i.accessTTL
	if typ == RefreshToken {
		ttl = i.refreshTTL
	}
	now := time.Now()
	claims := Claims{
		Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(i.priv)
}

// Parse verifies a token and returns its claims
func (i *Issuer) Parse(token string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, ErrInvalidToken
		}
		return i.pub, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	return c, nil
}

// GenerateKey returns a new PEM PKCS8 ed25519 private key
func GenerateKey() (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// LoadKey parses a PEM PKCS8 ed25519 key
func LoadKey(pemStr string) (ed25519.PrivateKey, error) {
	if pemStr == "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	}
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("auth: invalid pem")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("auth: not an ed25519 key")
	}
	return priv, nil
}

// LoadKeyFile reads and parses a FEM key from a file path
func LoadKeyFile(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("auth: read key file: %w", err)
	}
	return LoadKey(string(b))
}

// LoadKeyOrEphemeral loads a key from path, or generates an ephemeral one when
// path is empty (dev only). Bool reports whether key is ephemeral
func LoadKeyOrEphemeral(path string) (ed25519.PrivateKey, bool, error) {
	if path == "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, true, err
	}
	priv, err := LoadKeyFile(path)
	return priv, false, err
}
