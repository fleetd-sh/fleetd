package ferrors

import (
	"errors"
	"fmt"
)

// Simplified error handling for fleetd

// Common error variables
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidData  = errors.New("invalid data")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInternal     = errors.New("internal error")
)

// Wrap wraps an error with a message
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Wrapf wraps an error with a formatted message
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}

// New creates a new error
func New(msg string) error {
	return errors.New(msg)
}

// Is checks if an error matches a target
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As extracts an error of a specific type
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}