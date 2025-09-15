package repository

import "errors"

// Common repository errors
var (
	// ErrNotFound is returned when a requested resource is not found
	ErrNotFound = errors.New("resource not found")

	// ErrDuplicate is returned when attempting to create a duplicate resource
	ErrDuplicate = errors.New("resource already exists")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.New("invalid input")

	// ErrDatabase is returned for general database errors
	ErrDatabase = errors.New("database error")
)
