package auth

import (
	"testing"
	"time"
)

func testIssuer(t *testing.T, access, refresh time.Duration) *Issuer {
	t.Helper()
	priv, err := LoadKey("") // empty pem -> ephemeral ed25519 key
	if err != nil {
		t.Fatal(err)
	}
	return NewIssuer(priv, access, refresh)
}

func TestIssueParseAccess(t *testing.T) {
	iss := testIssuer(t, 15*time.Minute, time.Hour)
	tok, err := iss.Issue(4242, AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := iss.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Type != AccessToken {
		t.Fatalf("type = %q, want access", claims.Type)
	}
	if claims.Subject != "4242" {
		t.Fatalf("subject = %q, want 4242", claims.Subject)
	}
}

func TestIssueParseRefresh(t *testing.T) {
	iss := testIssuer(t, 15*time.Minute, time.Hour)
	tok, _ := iss.Issue(1, RefreshToken)
	claims, err := iss.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Type != RefreshToken {
		t.Fatalf("type = %q, want refresh", claims.Type)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	iss := testIssuer(t, -time.Minute, time.Hour) // issued already expired
	tok, _ := iss.Issue(1, AccessToken)
	if _, err := iss.Parse(tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestWrongKeyRejected(t *testing.T) {
	a := testIssuer(t, time.Hour, time.Hour)
	b := testIssuer(t, time.Hour, time.Hour)
	tok, _ := a.Issue(1, AccessToken)
	if _, err := b.Parse(tok); err == nil {
		t.Fatal("expected token signed by another key to be rejected")
	}
}

func TestTamperedTokenRejected(t *testing.T) {
	iss := testIssuer(t, time.Hour, time.Hour)
	tok, _ := iss.Issue(1, AccessToken)
	bad := tok[:len(tok)-1]
	if tok[len(tok)-1] == 'a' {
		bad += "b"
	} else {
		bad += "a"
	}
	if _, err := iss.Parse(bad); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestKeyRoundTrip(t *testing.T) {
	pem, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	priv, err := LoadKey(pem)
	if err != nil {
		t.Fatal(err)
	}
	iss := NewIssuer(priv, time.Hour, time.Hour)
	tok, _ := iss.Issue(7, AccessToken)
	if _, err := iss.Parse(tok); err != nil {
		t.Fatalf("parse after key round trip: %v", err)
	}
}

func TestLoadKeyRejectsGarbage(t *testing.T) {
	if _, err := LoadKey("not pem"); err == nil {
		t.Fatal("expected error for invalid pem")
	}
}
