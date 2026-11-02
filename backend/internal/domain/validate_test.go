package domain

import (
	"strings"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	cases := map[string]bool{
		"alice":                 true,
		"bob_42":                true,
		"ab":                    false,
		strings.Repeat("a", 33): false,
		"with space":            false,
		"кирилл":                false,
		"drop;table":            false,
	}
	for in, ok := range cases {
		if err := ValidateUsername(in); (err == nil) != ok {
			t.Errorf("ValidateUsername(%q) = %v, want ok=%v", in, err, ok)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"short":                 false,
		"long enough":           true,
		strings.Repeat("a", 72): true,
		strings.Repeat("a", 73): false, // bcrypt would silently truncate it
	}
	for in, ok := range cases {
		if err := ValidatePassword(in); (err == nil) != ok {
			t.Errorf("ValidatePassword(len=%d) = %v, want ok=%v", len(in), err, ok)
		}
	}
}

func TestNormalizeMessageBody(t *testing.T) {
	if got, err := NormalizeMessageBody("  hi  "); err != nil || got != "hi" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, in := range []string{"", "   \n\t", strings.Repeat("я", MessageMaxLen+1), "\xff\xfe"} {
		if _, err := NormalizeMessageBody(in); err == nil {
			t.Errorf("NormalizeMessageBody(%.10q) accepted", in)
		}
	}
	if _, err := NormalizeMessageBody(strings.Repeat("я", MessageMaxLen)); err != nil {
		t.Errorf("max-length message rejected: %v", err)
	}
}
