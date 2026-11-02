package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var secret = []byte("0123456789abcdef0123456789abcdef")

func TestTokenRoundTrip(t *testing.T) {
	m := NewTokenManager(secret, time.Hour)
	id := uuid.New()
	tok, exp, err := m.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) <= 0 {
		t.Fatalf("expiry in the past: %v", exp)
	}
	got, err := m.Parse(tok)
	if err != nil || got != id {
		t.Fatalf("Parse() = %v, %v; want %v", got, err, id)
	}
}

func TestTokenRejected(t *testing.T) {
	m := NewTokenManager(secret, time.Hour)
	valid, _, _ := m.Issue(uuid.New())

	expired := NewTokenManager(secret, time.Hour)
	expired.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expiredTok, _, _ := expired.Issue(uuid.New())

	otherKey, _, _ := NewTokenManager([]byte("another-secret-another-secret-xx"), time.Hour).Issue(uuid.New())

	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject: uuid.NewString(), Issuer: tokenIssuer, Audience: jwt.ClaimStrings{tokenAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	cases := map[string]string{
		"expired":   expiredTok,
		"wrong key": otherKey,
		"alg none":  none,
		"tampered":  valid[:len(valid)-2] + "xx",
		"garbage":   "not-a-jwt",
		"empty":     "",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := m.Parse(tok); err != ErrInvalidToken {
				t.Fatalf("Parse() err = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestTicketSingleUse(t *testing.T) {
	s := NewTicketStore(time.Minute)
	id := uuid.New()
	tk, _ := s.Issue(id)

	if got, ok := s.Consume(tk); !ok || got != id {
		t.Fatalf("first Consume() = %v, %v", got, ok)
	}
	if _, ok := s.Consume(tk); ok {
		t.Fatal("ticket accepted twice")
	}
}

func TestTicketExpires(t *testing.T) {
	s := NewTicketStore(time.Second)
	now := time.Now()
	s.now = func() time.Time { return now }
	tk, _ := s.Issue(uuid.New())

	now = now.Add(2 * time.Second)
	if _, ok := s.Consume(tk); ok {
		t.Fatal("expired ticket accepted")
	}
}

func TestPasswordHasher(t *testing.T) {
	h, err := NewPasswordHasher(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := h.Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse" {
		t.Fatal("password stored in plain text")
	}
	if !h.Compare(hash, "correct horse") {
		t.Fatal("valid password rejected")
	}
	if h.Compare(hash, "wrong horse") {
		t.Fatal("invalid password accepted")
	}
}
