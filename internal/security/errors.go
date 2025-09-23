package security

import "errors"

var (
	// ErrInvalidToken indicates an invalid token format
	ErrInvalidToken = errors.New("invalid token")

	// ErrTokenExpired indicates the token has expired
	ErrTokenExpired = errors.New("token expired")

	// ErrTokenRevoked indicates the token has been revoked
	ErrTokenRevoked = errors.New("token revoked")

	// ErrInvalidCredentials indicates invalid login credentials
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidSigningMethod indicates an unsupported signing method
	ErrInvalidSigningMethod = errors.New("invalid signing method")

	// ErrMissingClaims indicates required claims are missing
	ErrMissingClaims = errors.New("missing required claims")

	// ErrInvalidAudience indicates token audience doesn't match
	ErrInvalidAudience = errors.New("invalid audience")

	// ErrInvalidIssuer indicates token issuer doesn't match
	ErrInvalidIssuer = errors.New("invalid issuer")
)
