package main

import (
	"log"
	"net/http"
	"time"

	"github.com/Omar2709/pulseops/internal/httpapi"
	"github.com/Omar2709/pulseops/internal/trading"
)

func main() {
	// Create the shared trading service.
	tradingService := trading.NewService()

	// Configure the HTTP router.
	router := httpapi.NewRouter(tradingService)

	// Configure the HTTP server.
	server := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("PulseOps API listening on :8080")

	// Start the HTTP server.
	if err := server.ListenAndServe(); err != nil &&
		err != http.ErrServerClosed {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
