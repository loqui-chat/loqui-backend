// Command genkey prints a new PEM ed25519 private key for JWT signing
package main

import (
	"fmt"
	"os"

	"github.com/loqui-chat/loqui-backend/internal/auth"
)

func main() {
	key, err := auth.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genkey:", err)
		os.Exit(1)
	}
	fmt.Print(key)
}
