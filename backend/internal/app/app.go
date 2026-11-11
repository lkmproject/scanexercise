package app

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gin-gonic/gin"

	"scanexercise/internal/auth"
	"scanexercise/internal/config"
	"scanexercise/internal/editor"
	"scanexercise/internal/jobs"
	"scanexercise/internal/server"
)

type App struct {
	Engine *gin.Engine
	Store  *jobs.Store
}

func New() *App {
	settings := config.Load()
	ed := editor.NewTextIn(
		settings.TextInAppID,
		settings.TextInSecretCode,
		settings.TextInTimeoutSeconds,
	)
	awsCfg, awsOK := loadAWS(settings)
	store := jobs.NewStore(settings, ed)
	if awsOK {
		store = jobs.NewAWSStore(settings, ed, awsCfg)
		log.Printf("jobs aws table=%s bucket=%s project=%s", jobs.JobsTable, jobs.JobsBucket, jobs.JobsProject)
		if name := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); name != "" {
			store.SetAsyncStart(jobs.NewLambdaStarter(name, awsCfg))
			log.Printf("jobs async lambda=%s", name)
		}
	} else if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		log.Fatal("jobs aws required on lambda")
	} else {
		store.Cleanup()
		go func() {
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				store.Cleanup()
			}
		}()
	}

	accounts := newAuth(settings, awsCfg, awsOK)
	store.SetWallet(accounts)
	tokens := auth.NewTokens(settings.JWTSecret)
	if tokens == nil {
		log.Printf("jwt disabled: missing JWT_SECRET")
	} else {
		log.Printf("jwt enabled ttl=%s", auth.TokenTTL)
	}
	return &App{Engine: server.New(store, accounts, tokens), Store: store}
}

func loadAWS(settings config.Settings) (aws.Config, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(settings.AWSRegion))
	if err != nil {
		log.Printf("aws disabled: %v", err)
		return aws.Config{}, false
	}
	if _, err := awsCfg.Credentials.Retrieve(ctx); err != nil {
		log.Printf("aws disabled: %v", err)
		return aws.Config{}, false
	}
	return awsCfg, true
}

func newAuth(settings config.Settings, awsCfg aws.Config, awsOK bool) *auth.Service {
	if !awsOK {
		return nil
	}
	log.Printf("google auth enabled table=%s region=%s", auth.UsersTable, settings.AWSRegion)
	directory := auth.NewDynamoDirectory(dynamodb.NewFromConfig(awsCfg))
	return auth.NewService(auth.NewGoogleVerifier(settings.GoogleWebClientID), directory)
}
