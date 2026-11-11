package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"

	"scanexercise/internal/app"
)

var (
	instance  *app.App
	ginLambda *ginadapter.GinLambdaV2
)

func init() {
	if os.Getenv("DATA_DIR") == "" {
		_ = os.Setenv("DATA_DIR", "/tmp")
	}
	instance = app.New()
	ginLambda = ginadapter.NewV2(instance.Engine)
	log.Printf("lambda gin adapter ready")
}

func handler(ctx context.Context, event json.RawMessage) (any, error) {
	var probe struct {
		JobID          string          `json:"job_id"`
		RequestContext json.RawMessage `json:"requestContext"`
	}
	if err := json.Unmarshal(event, &probe); err != nil {
		return nil, err
	}
	if probe.JobID != "" && len(probe.RequestContext) == 0 {
		log.Printf("run job=%s", probe.JobID)
		instance.Store.Run(probe.JobID)
		return map[string]string{"status": "ok"}, nil
	}
	var req events.APIGatewayV2HTTPRequest
	if err := json.Unmarshal(event, &req); err != nil {
		return nil, err
	}
	return ginLambda.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(handler)
}
