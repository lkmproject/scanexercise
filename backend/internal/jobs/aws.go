package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	JobsProject = "scanexercise"
	JobsTable   = "Jobs"
	JobsBucket  = "scanexercise"
	JobsType    = "handwritten_erase"
)

type awsStore struct {
	ddb    *dynamodb.Client
	s3     *s3.Client
	table  string
	bucket string
}

type jobItem struct {
	PK        string  `dynamodbav:"pk"`
	SK        string  `dynamodbav:"sk"`
	Project   string  `dynamodbav:"project"`
	JobID     string  `dynamodbav:"jobId"`
	Type      string  `dynamodbav:"type"`
	UserID    string  `dynamodbav:"userId"`
	Status    string  `dynamodbav:"status"`
	PageCount int     `dynamodbav:"pageCount"`
	Crop      int     `dynamodbav:"crop"`
	Charged   int     `dynamodbav:"charged"`
	Refunded  bool    `dynamodbav:"refunded"`
	Error     *string `dynamodbav:"error"`
	CreatedAt string  `dynamodbav:"createdAt"`
	ExpiresAt string  `dynamodbav:"expiresAt"`
	TTL       int64   `dynamodbav:"ttl"`
}

func newAWSStore(cfg aws.Config) *awsStore {
	return &awsStore{
		ddb:    dynamodb.NewFromConfig(cfg),
		s3:     s3.NewFromConfig(cfg),
		table:  JobsTable,
		bucket: JobsBucket,
	}
}

func jobPK(id string) string {
	return "PROJECT#" + JobsProject + "#JOB#" + id
}

func s3Key(jobID, rel string) string {
	return "jobs/" + jobID + "/" + strings.TrimPrefix(rel, "/")
}

func (a *awsStore) writeMeta(ctx context.Context, job *Job) error {
	item, err := attributevalue.MarshalMap(toItem(job))
	if err != nil {
		return err
	}
	_, err = a.ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(a.table),
		Item:      item,
	})
	return err
}

func (a *awsStore) readMeta(ctx context.Context, jobID string) (*Job, error) {
	out, err := a.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(a.table),
		Key: map[string]ddbtypes.AttributeValue{
			"pk": &ddbtypes.AttributeValueMemberS{Value: jobPK(jobID)},
			"sk": &ddbtypes.AttributeValueMemberS{Value: "META"},
		},
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}
	var item jobItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, err
	}
	return fromItem(item), nil
}

func (a *awsStore) writeBlob(ctx context.Context, jobID, rel string, data []byte) error {
	contentType := "application/octet-stream"
	switch {
	case strings.HasSuffix(rel, ".jpg"):
		contentType = "image/jpeg"
	case strings.HasSuffix(rel, ".pdf"):
		contentType = "application/pdf"
	}
	_, err := a.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.bucket),
		Key:         aws.String(s3Key(jobID, rel)),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	return err
}

func (a *awsStore) readBlob(ctx context.Context, jobID, rel string) ([]byte, error) {
	out, err := a.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(s3Key(jobID, rel)),
	})
	if err != nil {
		var notFound *s3types.NoSuchKey
		if errors.As(err, &notFound) || isS3NotFound(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (a *awsStore) remove(ctx context.Context, jobID string) error {
	if err := a.deletePrefix(ctx, s3Key(jobID, "")); err != nil {
		return err
	}
	_, err := a.ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(a.table),
		Key: map[string]ddbtypes.AttributeValue{
			"pk": &ddbtypes.AttributeValueMemberS{Value: jobPK(jobID)},
			"sk": &ddbtypes.AttributeValueMemberS{Value: "META"},
		},
	})
	return err
}

func (a *awsStore) removeBlobs(ctx context.Context, jobID string, rels ...string) error {
	for _, rel := range rels {
		if strings.HasSuffix(rel, "/") {
			if err := a.deletePrefix(ctx, s3Key(jobID, rel)); err != nil {
				return err
			}
			continue
		}
		_, err := a.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(s3Key(jobID, rel)),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *awsStore) listIDs(context.Context) []string {
	return nil
}

func (a *awsStore) deletePrefix(ctx context.Context, prefix string) error {
	var token *string
	for {
		out, err := a.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(a.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return err
		}
		if len(out.Contents) == 0 {
			return nil
		}
		objects := make([]s3types.ObjectIdentifier, 0, len(out.Contents))
		for _, obj := range out.Contents {
			objects = append(objects, s3types.ObjectIdentifier{Key: obj.Key})
		}
		_, err = a.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(a.bucket),
			Delete: &s3types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
		if aws.ToBool(out.IsTruncated) {
			token = out.NextContinuationToken
			continue
		}
		return nil
	}
}

func toItem(job *Job) jobItem {
	ttl := time.Now().UTC().Add(24 * time.Hour).Unix()
	if expires, err := parseTime(job.ExpiresAt); err == nil {
		ttl = expires.Unix()
	}
	return jobItem{
		PK:        jobPK(job.ID),
		SK:        "META",
		Project:   JobsProject,
		JobID:     job.ID,
		Type:      JobsType,
		UserID:    job.UserID,
		Status:    job.Status,
		PageCount: job.PageCount,
		Crop:      job.Crop,
		Charged:   job.Charged,
		Refunded:  job.Refunded,
		Error:     job.Error,
		CreatedAt: job.CreatedAt,
		ExpiresAt: job.ExpiresAt,
		TTL:       ttl,
	}
}

func fromItem(item jobItem) *Job {
	return &Job{
		ID:        item.JobID,
		UserID:    item.UserID,
		Status:    item.Status,
		PageCount: item.PageCount,
		Crop:      item.Crop,
		Charged:   item.Charged,
		Refunded:  item.Refunded,
		Error:     item.Error,
		CreatedAt: item.CreatedAt,
		ExpiresAt: item.ExpiresAt,
	}
}

func isS3NotFound(err error) bool {
	var nsk *s3types.NotFound
	if errors.As(err, &nsk) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "NoSuchKey") || strings.Contains(msg, "NotFound")
}
