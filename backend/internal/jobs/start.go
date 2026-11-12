package jobs

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

type RunEvent struct {
	JobID string `json:"job_id"`
}

func NewLambdaStarter(functionName string, cfg aws.Config) func(jobID string) error {
	client := lambda.NewFromConfig(cfg)
	return func(jobID string) error {
		payload, err := json.Marshal(RunEvent{JobID: jobID})
		if err != nil {
			return err
		}
		log.Printf("async invoke job=%s fn=%s", jobID, functionName)
		_, err = client.Invoke(context.Background(), &lambda.InvokeInput{
			FunctionName:   aws.String(functionName),
			InvocationType: types.InvocationTypeEvent,
			Payload:        payload,
		})
		if err != nil {
			log.Printf("async invoke failed job=%s: %v", jobID, err)
			return err
		}
		log.Printf("async invoke ok job=%s", jobID)
		return nil
	}
}
