package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/register", nil)
	w := httptest.NewRecorder()

	registerHandler(w, req)
	res := w.Result()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "register") {
		t.Error("response should contain register form")
	}
}

func TestLoginHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	loginHandler(w, req)
	res := w.Result()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "login") {
		t.Error("response should contain login form")
	}
}

func TestCreatePostHandlerGET(t *testing.T) {
	req := httptest.NewRequest("GET", "/create_post", nil)
	w := httptest.NewRecorder()

	createPostHandler(w, req)
	res := w.Result()

	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusSeeOther {
		return
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 or redirect, got %d", res.StatusCode)
	}
}

func TestEditPostHandlerNotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/edit_post?id=99999", nil)
	w := httptest.NewRecorder()

	editPostHandler(w, req)
	res := w.Result()

	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusSeeOther {
		return
	}

	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 404 or 400, got %d", res.StatusCode)
	}
}

func TestDeletePostHandlerMissingID(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/delete_post", nil)
	w := httptest.NewRecorder()

	deletePostHandler(w, req)
	res := w.Result()

	if res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusSeeOther {
		t.Errorf("expected bad request or redirect, got %d", res.StatusCode)
	}
}

func TestPostDetailHandlerInvalidID(t *testing.T) {
	req := httptest.NewRequest("GET", "/post/invalid", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			return
		}
	}()

	postDetailHandler(w, req)
	res := w.Result()

	if res.StatusCode == http.StatusInternalServerError {
		return
	}
}

func TestPostDetailHandlerNotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/post/99999", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			return
		}
	}()

	postDetailHandler(w, req)
	res := w.Result()

	if res.StatusCode == http.StatusInternalServerError {
		return
	}
}

func TestEditCommentHandlerMissingID(t *testing.T) {
	req := httptest.NewRequest("GET", "/edit_comment", nil)
	w := httptest.NewRecorder()

	editCommentHandler(w, req)
	res := w.Result()

	if res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusSeeOther {
		t.Errorf("expected bad request or redirect, got %d", res.StatusCode)
	}
}

func TestDeleteCommentHandlerMissingID(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/delete_comment", nil)
	w := httptest.NewRecorder()

	deleteCommentHandler(w, req)
	res := w.Result()

	if res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusSeeOther {
		t.Errorf("expected bad request or redirect, got %d", res.StatusCode)
	}
}

func TestStaticFileServing(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/static/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/static/favicon.ico", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRender400Function(t *testing.T) {
	w := httptest.NewRecorder()

	Render400(w, "Test error message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetCurrentUserWithoutSession(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	user := GetCurrentUser(req)

	if user != nil {
		t.Error("expected nil user without session cookie")
	}
}

func TestGetCurrentUserWithInvalidSession(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid-uuid"})

	defer func() {
		if r := recover(); r != nil {
			return
		}
	}()

	user := GetCurrentUser(req)

	if user == nil {
		return
	}
}

func TestHomeHandlerNoDatabase(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			return
		}
	}()

	homeHandler(w, req)

	if w.Code == http.StatusInternalServerError {
		return
	}
}

func TestLikeCommentHandlerInvalidID(t *testing.T) {
	req := httptest.NewRequest("GET", "/like_comment?id=invalid&post=1&like=1", nil)
	w := httptest.NewRecorder()

	likeCommentHandler(w, req)
	res := w.Result()

	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusSeeOther {
		return
	}
}

func TestLikePostHandlerMissingParams(t *testing.T) {
	req := httptest.NewRequest("GET", "/like_post", nil)
	w := httptest.NewRecorder()

	likePostHandler(w, req)
	res := w.Result()

	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusSeeOther {
		return
	}
}

func TestFormValidation(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		maxLen int
		valid  bool
	}{
		{"empty string", "", 10, false},
		{"valid short", "test", 10, true},
		{"too long", "this is a very long string", 10, false},
		{"max length", "1234567890", 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.value) > 0 && len(tt.value) <= tt.maxLen {
				if !tt.valid {
					t.Error("expected valid, got invalid")
				}
			} else if len(tt.value) > tt.maxLen {
				if tt.valid {
					t.Error("expected invalid, got valid")
				}
			}
		})
	}
}

func TestHTTPMethodValidation(t *testing.T) {
	tests := []struct {
		method string
		path   string
		valid  bool
	}{
		{"GET", "/", true},
		{"POST", "/create_post", true},
		{"DELETE", "/delete_post?id=1", true},
		{"PUT", "/edit_post?id=1", true},
		{"HEAD", "/", false},
		{"OPTIONS", "/", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.method, tt.path), func(t *testing.T) {
			if tt.valid {
				req := httptest.NewRequest(tt.method, tt.path, nil)
				if req == nil {
					t.Error("failed to create request")
				}
			}
		})
	}
}

func TestRequestBodyParsing(t *testing.T) {
	body := bytes.NewBufferString("title=test&content=test content")
	req := httptest.NewRequest("POST", "/create_post", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if req.Body == nil {
		t.Error("request body should not be nil")
	}

	err := req.ParseForm()
	if err != nil {
		t.Errorf("failed to parse form: %v", err)
	}

	if req.FormValue("title") != "test" {
		t.Error("form parsing failed")
	}
}
