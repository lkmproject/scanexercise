package jobs

import (
	"context"
	"encoding/json"

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
		_, err = client.Invoke(context.Background(), &lambda.InvokeInput{
			FunctionName:   aws.String(functionName),
			InvocationType: types.InvocationTypeEvent,
			Payload:        payload,
		})
		return err
	}
}
