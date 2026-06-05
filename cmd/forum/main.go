package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"forum/internal/api"
	"forum/internal/auth"
	"forum/internal/cache"
	"forum/internal/config"
	"forum/internal/database"
	"forum/internal/handlers"
	"forum/internal/metrics"
	"forum/internal/storage"
	ws "forum/internal/websocket"
)

func main() {
	cfg := config.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	metrics.Init()

	// ---- Database ----
	var store database.Store
	if cfg.UsePostgres() {
		db, err := database.InitPostgres(cfg.Database.DSN)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		db.SetMaxOpenConns(cfg.Database.MaxOpenConn)
		db.SetMaxIdleConns(cfg.Database.MaxIdleConn)
		db.SetConnMaxLifetime(cfg.Database.ConnMaxLife)
		store = database.NewPostgresStore(db, logger)
		log.Println("Using PostgreSQL")
	} else {
		db, err := database.InitSQLite(cfg.Database.Path)
		if err != nil {
			log.Fatalf("sqlite: %v", err)
		}
		store = database.NewSQLiteStore(db, logger)
		log.Println("Using SQLite")
	}
	defer func() { _ = store.Close() }()

	// ---- JWT ----
	jwtManager := auth.NewManager(
		cfg.JWT.Secret,
		cfg.JWT.AccessTokenDuration,
		cfg.JWT.RefreshTokenDuration,
	)

	// ---- Redis (optional, graceful degradation) ----
	var redisCache *cache.Cache
	if c, err := cache.New(cfg.Redis.URL); err == nil {
		redisCache = c
		defer func() { _ = redisCache.Close() }()
		log.Println("Redis connected")
	} else {
		logger.Warn("redis unavailable, running without cache/rate-limit", slog.Any("err", err))
	}

	// ---- MinIO (optional, graceful degradation) ----
	var storageClient *storage.Client
	if sc, err := storage.New(
		cfg.MinIO.Endpoint,
		cfg.MinIO.AccessKeyID,
		cfg.MinIO.SecretAccessKey,
		cfg.MinIO.BucketName,
		cfg.MinIO.UseSSL,
	); err == nil {
		storageClient = sc
		log.Println("MinIO connected")
	} else {
		logger.Warn("minio unavailable, file uploads disabled", slog.Any("err", err))
	}

	// ---- WebSocket Hub ----
	hub := ws.NewHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// ---- HTTP mux ----
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.Handle("/metrics", metrics.Handler())

	// REST API v1 (handles /api/v1/*)
	apiServer := api.NewServer(store, jwtManager, redisCache, storageClient, hub)
	apiServer.Mount(mux)

	// HTML handlers (handles /, /post/*, /login, etc.)
	handlers.RegisterRoutes(mux, store)

	srv := &http.Server{
		Addr:         cfg.GetServerAddr(),
		Handler:      metrics.Middleware(mux),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	log.Printf("Forum started on %s", cfg.GetServerAddr())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
