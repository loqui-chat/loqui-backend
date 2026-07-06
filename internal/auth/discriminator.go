package auth

import "crypto/rand"

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// DisriminatorLen is the fixed discriminator length
const DisriminatorLen = 4

// NewDiscriminator returns a random 4 char base62 string (unbiased)
func NewDiscriminator() (string, error) {
	out := make([]byte, DisriminatorLen)
	buf := make([]byte, 1)
	for i := 0; i < DisriminatorLen; {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		// reject top bytes so modulo stays unbiased (248 = 62*4)
		if buf[0] >= 248 {
			continue
		}
		out[i] = base62[buf[0]%62]
		i++
	}
	return string(out), nil
}
