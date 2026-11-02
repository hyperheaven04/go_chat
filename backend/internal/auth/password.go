// Package auth provides password hashing, JWT access tokens and
// one-time WebSocket tickets.
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type PasswordHasher struct {
	cost int
	// dummyHash is compared against when a user does not exist, so that
	// login takes the same time either way and usernames cannot be
	// enumerated through response timing.
	dummyHash []byte
}

func NewPasswordHasher(cost int) (*PasswordHasher, error) {
	dummy, err := bcrypt.GenerateFromPassword([]byte("timing-equalization"), cost)
	if err != nil {
		return nil, fmt.Errorf("generate dummy hash: %w", err)
	}
	return &PasswordHasher{cost: cost, dummyHash: dummy}, nil
}

func (h *PasswordHasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

func (h *PasswordHasher) Compare(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CompareDummy burns the same CPU time as a real comparison.
func (h *PasswordHasher) CompareDummy(password string) {
	_ = bcrypt.CompareHashAndPassword(h.dummyHash, []byte(password))
}
