package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"forum/internal/api"
	"forum/internal/auth"
	"forum/internal/config"
	"forum/internal/database"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupPostgres starts a throwaway PostgreSQL container and returns the DSN.
func setupPostgres(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("forum_test"),
		postgres.WithUsername("forum"),
		postgres.WithPassword("forum"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Skipf("testcontainers not available: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	return dsn, func() {
		_ = container.Terminate(ctx)
	}
}

func setupServer(t *testing.T, store database.Store) *httptest.Server {
	t.Helper()
	cfg := config.LoadConfig()
	jwtManager := auth.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTokenDuration, cfg.JWT.RefreshTokenDuration)

	mux := http.NewServeMux()
	apiServer := api.NewServer(store, jwtManager, nil, nil, nil)
	apiServer.Mount(mux)

	return httptest.NewServer(mux)
}

// TestIntegration_RegisterLoginPosts runs the full CRUD flow against a real PostgreSQL container.
func TestIntegration_RegisterLoginPosts(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}

	dsn, cleanup := setupPostgres(t)
	defer cleanup()

	db, err := database.InitPostgres(dsn)
	if err != nil {
		t.Fatalf("postgres init: %v", err)
	}

	store := database.NewPostgresStore(db, nil)
	defer store.Close()

	srv := setupServer(t, store)
	defer srv.Close()
	base := srv.URL

	// ---- Register ----
	regBody, _ := json.Marshal(map[string]string{
		"email": "test@example.com", "username": "testuser", "password": "password123",
	})
	resp, err := http.Post(base+"/api/v1/auth/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", resp.StatusCode)
	}
	var regResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&regResp)
	accessToken, _ := regResp["access_token"].(string)
	if accessToken == "" {
		t.Fatal("no access token in register response")
	}
	t.Logf("registered user, token: %s...", accessToken[:20])

	// ---- Login ----
	loginBody, _ := json.Marshal(map[string]string{"login": "testuser", "password": "password123"})
	resp, err = http.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: want 200, got %d", resp.StatusCode)
	}

	// ---- Get categories ----
	resp, err = http.Get(base + "/api/v1/categories")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("categories: want 200, got %d", resp.StatusCode)
	}
	var cats []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&cats)
	if len(cats) == 0 {
		t.Fatal("expected at least one category")
	}
	catID := int(cats[0]["id"].(float64))

	// ---- Create post (authenticated) ----
	postBody, _ := json.Marshal(map[string]interface{}{
		"title": "Hello World", "content": "Test content", "category_ids": []int{catID},
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/posts", bytes.NewReader(postBody))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create post: want 201, got %d", resp.StatusCode)
	}
	var postResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&postResp)
	postID := int(postResp["id"].(float64))
	t.Logf("created post id=%d", postID)

	// ---- List posts ----
	resp, err = http.Get(base + "/api/v1/posts")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list posts: want 200, got %d", resp.StatusCode)
	}

	// ---- Get single post ----
	resp, err = http.Get(fmt.Sprintf("%s/api/v1/posts/%d", base, postID))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get post: want 200, got %d", resp.StatusCode)
	}

	// ---- Search ----
	resp, err = http.Get(base + "/api/v1/search?q=Hello")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: want 200, got %d", resp.StatusCode)
	}

	// ---- Delete post ----
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/posts/%d", base, postID), nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete post: want 204, got %d", resp.StatusCode)
	}
	t.Log("integration test passed")
}

// TestIntegration_SQLite_CRUD runs the same flow against SQLite (fast, no containers needed).
func TestIntegration_SQLite_CRUD(t *testing.T) {
	db, err := database.InitSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store := database.NewSQLiteStoreWithoutLogger(db)
	defer store.Close()

	srv := setupServer(t, store)
	defer srv.Close()
	base := srv.URL

	// Register
	regBody, _ := json.Marshal(map[string]string{
		"email": "u@test.com", "username": "user1", "password": "pass1234",
	})
	resp, err := http.Post(base+"/api/v1/auth/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: got %d", resp.StatusCode)
	}

	var regResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&regResp)
	token, _ := regResp["access_token"].(string)

	// Categories
	resp, _ = http.Get(base + "/api/v1/categories")
	var cats []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&cats)
	catID := int(cats[0]["id"].(float64))

	// Create post
	postBody, _ := json.Marshal(map[string]interface{}{
		"title": "SQLite Test", "content": "Works!", "category_ids": []int{catID},
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/posts", bytes.NewReader(postBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create post: got %d", resp.StatusCode)
	}

	// Health check
	resp, _ = http.Get(base + "/api/v1/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: got %d", resp.StatusCode)
	}
}
