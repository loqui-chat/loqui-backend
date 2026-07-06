package auth

import (
	"strings"
	"testing"
)

func TestDiscriminatorFormat(t *testing.T) {
	for range 20000 {
		d, err := NewDiscriminator()
		if err != nil {
			t.Fatal(err)
		}
		if len(d) != DisriminatorLen {
			t.Fatalf("len = %d, want %d", len(d), DisriminatorLen)
		}
		for _, c := range d {
			if !strings.ContainsRune(base62, c) {
				t.Fatalf("char %q not in base62", c)
			}
		}
	}
}

func TestDiscriminatorSpread(t *testing.T) {
	// every base62 char should appear at least once over many draws
	seen := map[rune]bool{}
	for range 50000 {
		d, _ := NewDiscriminator()
		for _, c := range d {
			seen[c] = true
		}
	}
	if len(seen) != len(base62) {
		t.Fatalf("only %d of %d chars seen", len(seen), len(base62))
	}
}
