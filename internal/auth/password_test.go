package auth

import "testing"

func TestHashVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("correct horse battery staple", h)
	if err != nil || !ok {
		t.Fatalf("expected match, ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong", h)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected mismatch")
	}
}

func TestHashUniqueSalt(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("hashes should differ due to random salt")
	}
}

func TestVerifyBadHash(t *testing.T) {
	if _, err := VerifyPassword("x", "not-a-hash"); err != ErrBadHash {
		t.Fatalf("expected ErrBadHash, got %v", err)
	}
}
