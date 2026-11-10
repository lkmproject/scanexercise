package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"scanexercise/internal/auth"
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

	accounts := newAuth(settings)
	store.SetWallet(accounts)
	tokens := auth.NewTokens(settings.JWTSecret)
	if tokens == nil {
		log.Printf("jwt disabled: missing JWT_SECRET")
	} else {
		log.Printf("jwt enabled ttl=%s", auth.TokenTTL)
	}
	addr := ":" + getenv("PORT", "8000")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, server.New(store, accounts, tokens)); err != nil {
		log.Fatal(err)
	}
}

func newAuth(settings config.Settings) *auth.Service {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(settings.AWSRegion))
	if err != nil {
		log.Printf("auth disabled: %v", err)
		return nil
	}
	if _, err := awsCfg.Credentials.Retrieve(ctx); err != nil {
		log.Printf("auth disabled: %v", err)
		return nil
	}
	log.Printf("google auth enabled table=%s region=%s", auth.UsersTable, settings.AWSRegion)
	directory := auth.NewDynamoDirectory(dynamodb.NewFromConfig(awsCfg))
	return auth.NewService(auth.NewGoogleVerifier(settings.GoogleWebClientID), directory)
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
