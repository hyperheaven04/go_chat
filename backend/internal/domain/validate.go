package domain

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	UsernameMinLen = 3
	UsernameMaxLen = 32
	PasswordMinLen = 8
	// bcrypt silently ignores everything after 72 bytes, so longer passwords
	// are rejected instead of being truncated without the user knowing.
	PasswordMaxBytes = 72
	MessageMaxLen    = 4000
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// NormalizeUsername makes usernames case-insensitive: "Alice" and "alice" are the same account.
func NormalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func ValidateUsername(s string) error {
	if n := utf8.RuneCountInString(s); n < UsernameMinLen || n > UsernameMaxLen {
		return NewValidationError("username", "must be 3-32 characters long")
	}
	if !usernameRe.MatchString(s) {
		return NewValidationError("username", "may contain only latin letters, digits and underscore")
	}
	return nil
}

func ValidatePassword(s string) error {
	if utf8.RuneCountInString(s) < PasswordMinLen {
		return NewValidationError("password", "must be at least 8 characters long")
	}
	if len(s) > PasswordMaxBytes {
		return NewValidationError("password", "must be at most 72 bytes long")
	}
	return nil
}

// NormalizeMessageBody trims surrounding whitespace and validates the result.
func NormalizeMessageBody(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", NewValidationError("body", "must be valid UTF-8")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", NewValidationError("body", "must not be empty")
	}
	if utf8.RuneCountInString(s) > MessageMaxLen {
		return "", NewValidationError("body", "must be at most 4000 characters long")
	}
	return s, nil
}
