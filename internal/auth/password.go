package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters tuned for login
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB (64 MB)
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

var ErrBadHash = errors.New("auth: malformed password hash")

// HashPassword retunrs a PHC-format argon2id hash
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports wether password matches encoded hash
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	other := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, other) == 1, nil
}

type params struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return params{}, nil, nil, ErrBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return params{}, nil, nil, ErrBadHash
	}
	var p params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return params{}, nil, nil, ErrBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return params{}, nil, nil, ErrBadHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return params{}, nil, nil, ErrBadHash
	}
	return p, salt, key, nil
}
