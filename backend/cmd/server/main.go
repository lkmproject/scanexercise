package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"scanexercise/internal/config"
	"scanexercise/internal/editor"
	"scanexercise/internal/jobs"
	"scanexercise/internal/server"
)

func main() {
	settings := config.Load()
	store := jobs.NewStore(settings, editor.NewTextIn(
		settings.TextInAppID,
		settings.TextInSecretCode,
		settings.TextInTimeoutSeconds,
	))
	store.Cleanup()
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			store.Cleanup()
		}
	}()

	addr := ":" + getenv("PORT", "8000")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, server.New(store)); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
