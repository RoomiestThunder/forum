package api

import (
	"errors"
	"log/slog"
	"net/http"

	apperrors "forum/internal/errors"
)

// storeError writes the response for an error returned by the store.
//
// An AppError below 500 carries a status and a message meant for the client
// (not found, forbidden, validation), so it is passed through. Everything else
// - a database failure, an unexpected error type - is an internal error: it is
// logged with enough request context to find, and the client gets only a
// generic "failed to <action>". The error text itself must never reach the
// client; for database errors it names tables, columns and driver internals.
func (s *Server) storeError(w http.ResponseWriter, r *http.Request, err error, action string) {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) && appErr.HTTPStatusCode() < http.StatusInternalServerError {
		message := appErr.Message
		if appErr.Details != "" {
			message = appErr.Details
		}
		jsonError(w, message, appErr.HTTPStatusCode())
		return
	}
	s.internalError(w, r, err, action)
}

// internalError logs err and answers 500 with a generic message.
func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error, action string) {
	s.logger.Error("request failed",
		slog.String("action", action),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Int("user_id", userIDFromCtx(r)),
		slog.Any("err", err),
	)
	jsonError(w, "failed to "+action, http.StatusInternalServerError)
}
