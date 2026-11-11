package database

import (
	"net/http"

	apperrors "forum/internal/errors"
)

// pgUniqueViolation is PostgreSQL's SQLSTATE for a unique_violation.
const pgUniqueViolation = "23505"

// errAccountTaken is what CreateUser returns when the email or username is
// already registered. Both stores map their own "unique constraint" error to
// it, so callers see a 409 and never a driver-specific failure.
func errAccountTaken() *apperrors.AppError {
	return apperrors.NewAppError(http.StatusConflict, "email or username already taken")
}
