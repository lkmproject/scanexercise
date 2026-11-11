package main

import (
	"log"
	"net/http"
	"os"

	"scanexercise/internal/app"
)

func main() {
	addr := ":" + getenv("PORT", "8000")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, app.New().Engine); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
