package errors

import (
	"net/http"
	"testing"
)

func TestErrorsStructure(t *testing.T) {
	appErr := NewAppError(400, "Test error")
	if appErr.Code != 400 {
		t.Errorf("Expected code 400, got %d", appErr.Code)
	}
	if appErr.Message != "Test error" {
		t.Errorf("Expected message 'Test error', got '%s'", appErr.Message)
	}
}

func TestErrorsWithDetails(t *testing.T) {
	appErr := NewAppError(400, "Bad request").WithDetails("Invalid email format")
	if appErr.Details != "Invalid email format" {
		t.Errorf("Expected details 'Invalid email format', got '%s'", appErr.Details)
	}
}

func TestErrorsWithError(t *testing.T) {
	err := NewAppError(500, "Database error").WithError(nil)
	if err.Err != nil {
		t.Errorf("Expected no underlying error, got %v", err.Err)
	}
}

func TestErrorHTTPStatusCode(t *testing.T) {
	tests := []struct {
		code         int
		expectedCode int
	}{
		{400, 400},
		{404, 404},
		{500, 500},
		{999, 500},
		{50, 500},
	}

	for _, tt := range tests {
		appErr := NewAppError(tt.code, "test")
		if appErr.HTTPStatusCode() != tt.expectedCode {
			t.Errorf("For code %d, expected %d, got %d", tt.code, tt.expectedCode, appErr.HTTPStatusCode())
		}
	}
}

func TestErrorStringMethods(t *testing.T) {
	appErr := NewAppError(400, "Bad input")
	errStr := appErr.Error()
	if errStr != "[400] Bad input" {
		t.Errorf("Expected '[400] Bad input', got '%s'", errStr)
	}
}

func TestCommonErrors(t *testing.T) {
	tests := []struct {
		name         string
		errFunc      func() *AppError
		expectedCode int
	}{
		{"NotFound", ErrNotFound, http.StatusNotFound},
		{"Unauthorized", ErrUnauthorized, http.StatusUnauthorized},
		{"Forbidden", ErrForbidden, http.StatusForbidden},
		{"BadRequest", ErrBadRequest, http.StatusBadRequest},
		{"Conflict", ErrConflict, http.StatusConflict},
		{"InternalServer", ErrInternalServer, http.StatusInternalServerError},
		{"TooManyRequests", ErrTooManyRequests, http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		appErr := tt.errFunc()
		if appErr.Code != tt.expectedCode {
			t.Errorf("%s: expected code %d, got %d", tt.name, tt.expectedCode, appErr.Code)
		}
	}
}

func TestInvalidInputError(t *testing.T) {
	err := InvalidInput("email", "missing @")
	if err.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", err.Code)
	}
	if err.Details == "" {
		t.Error("Expected details to be set")
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError("Username already exists")
	if err.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", err.Code)
	}
	if err.Details != "Username already exists" {
		t.Errorf("Expected 'Username already exists', got '%s'", err.Details)
	}
}

func TestNotFoundError(t *testing.T) {
	err := NotFoundError("Post")
	if err.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", err.Code)
	}
}

func TestPermissionError(t *testing.T) {
	err := PermissionError("delete")
	if err.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", err.Code)
	}
}

func TestDatabaseError(t *testing.T) {
	err := DatabaseError(nil)
	if err.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", err.Code)
	}
}
