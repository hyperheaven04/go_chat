package service

import (
	"context"
	"errors"
	"time"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

type AuthService struct {
	users  UserRepository
	hasher PasswordHasher
	tokens TokenIssuer
}

func NewAuthService(users UserRepository, hasher PasswordHasher, tokens TokenIssuer) *AuthService {
	return &AuthService{users: users, hasher: hasher, tokens: tokens}
}

type AuthResult struct {
	User      domain.User
	Token     string
	ExpiresAt time.Time
}

func (s *AuthService) Register(ctx context.Context, username, password string) (AuthResult, error) {
	username = domain.NormalizeUsername(username)
	if err := errors.Join(domain.ValidateUsername(username), domain.ValidatePassword(password)); err != nil {
		return AuthResult{}, firstValidationError(err)
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return AuthResult{}, err
	}
	u, err := s.users.Create(ctx, username, hash)
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(u)
}

// Login returns domain.ErrInvalidCredentials without saying whether the
// username or the password was wrong.
func (s *AuthService) Login(ctx context.Context, username, password string) (AuthResult, error) {
	u, hash, err := s.users.GetCredentials(ctx, domain.NormalizeUsername(username))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.hasher.CompareDummy(password)
		return AuthResult{}, domain.ErrInvalidCredentials
	case err != nil:
		return AuthResult{}, err
	}
	if !s.hasher.Compare(hash, password) {
		return AuthResult{}, domain.ErrInvalidCredentials
	}
	return s.issue(u)
}

func (s *AuthService) issue(u domain.User) (AuthResult, error) {
	token, exp, err := s.tokens.Issue(u.ID)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: u, Token: token, ExpiresAt: exp}, nil
}

func firstValidationError(err error) error {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		return ve
	}
	return err
}
