package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Omar2709/pulseops/internal/httpapi"
	"github.com/Omar2709/pulseops/internal/postgres"
	"github.com/Omar2709/pulseops/internal/trading"
)

func main() {
	tradingService := trading.NewService()

	databaseURL := strings.TrimSpace(
		os.Getenv("DATABASE_URL"),
	)

	if databaseURL != "" {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)

		pool, err := postgres.Open(ctx, databaseURL)
		if err != nil {
			cancel()
			log.Fatalf(
				"PostgreSQL connection failed: %v",
				err,
			)
		}

		if err := postgres.Migrate(ctx, pool); err != nil {
			cancel()
			pool.Close()
			log.Fatalf(
				"PostgreSQL migrations failed: %v",
				err,
			)
		}

		store, err := postgres.NewStore(pool)
		if err != nil {
			cancel()
			pool.Close()
			log.Fatalf(
				"PostgreSQL store initialization failed: %v",
				err,
			)
		}

		cancel()
		defer pool.Close()

		tradingService = trading.NewServiceWithStore(store)

		log.Println("PostgreSQL persistence enabled")
	} else {
		log.Println(
			"DATABASE_URL is not set; using in-memory persistence",
		)
	}

	router := httpapi.NewRouter(tradingService)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("PulseOps API listening on :8080")

	if err := server.ListenAndServe(); err != nil &&
		err != http.ErrServerClosed {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
