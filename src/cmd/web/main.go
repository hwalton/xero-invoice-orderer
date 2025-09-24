package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hwalton/freeride-campervans/internal/frontend"
	"github.com/hwalton/freeride-campervans/internal/handler"
	"github.com/hwalton/freeride-campervans/pkg/auth"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("no .env file found — relying on environment: %v", err)
	}

	addr := ":" + getEnv("PORT", "8080")
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Construct an authenticator (replace with your pkg/auth constructor)
	// e.g. auth.NewJWT(secret) or auth.NewSupabaseClient(supabaseURL, httpClient)
	authProvider := auth.NewJWT(
		os.Getenv("SUPABASE_JWT_SECRET"),
		os.Getenv("SUPABASE_JWT_ISSUER"),
		os.Getenv("SUPABASE_JWT_AUDIENCE"),
	)

	// Read DB url from env and pass to handler.NewRouter
	dbURL := getEnv("SUPABASE_URL", "")
	if dbURL == "" {
		log.Fatal("SUPABASE_URL environment variable is required")
	}

	tpls, err := frontend.BuildTemplates()
	if err != nil {
		log.Fatalf("build templates: %v", err)
	}
	appRouter := handler.NewRouter(authProvider, httpClient, dbURL, tpls)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Serve embedded static files at /static/*
	// frontend.StaticFS embeds files under the "static/" directory, so expose its "static" subtree.
	staticSub, err := fs.Sub(frontend.StaticFS, "static")
	if err != nil {
		log.Fatalf("failed to prepare static files: %v", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Mount application routes
	r.Mount("/", appRouter)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
		// optional: ReadTimeout, WriteTimeout, IdleTimeout
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("starting server on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
