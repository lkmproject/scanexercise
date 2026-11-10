package auth

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const UsersTable = "Users"

type DynamoDirectory struct {
	client *dynamodb.Client
	table  string
}

func NewDynamoDirectory(client *dynamodb.Client) *DynamoDirectory {
	return &DynamoDirectory{client: client, table: UsersTable}
}

type userItem struct {
	PK          string `dynamodbav:"pk"`
	SK          string `dynamodbav:"sk"`
	UserID      string `dynamodbav:"userId"`
	Email       string `dynamodbav:"email"`
	Name        string `dynamodbav:"name"`
	CreatedAt   string `dynamodbav:"createdAt"`
	LastLoginAt string `dynamodbav:"lastLoginAt"`
}

type linkItem struct {
	PK     string `dynamodbav:"pk"`
	SK     string `dynamodbav:"sk"`
	UserID string `dynamodbav:"userId"`
}

type balanceItem struct {
	PK        string `dynamodbav:"pk"`
	SK        string `dynamodbav:"sk"`
	Remaining int    `dynamodbav:"remaining"`
	UpdatedAt string `dynamodbav:"updatedAt"`
}

func (d *DynamoDirectory) FindByGoogle(ctx context.Context, subject string) (*User, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: googleLinkPK(subject)},
			"sk": &types.AttributeValueMemberS{Value: "LINK"},
		},
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}
	var link linkItem
	if err := attributevalue.UnmarshalMap(out.Item, &link); err != nil {
		return nil, err
	}
	if link.UserID == "" {
		return nil, nil
	}
	return d.Get(ctx, link.UserID)
}

func (d *DynamoDirectory) Create(ctx context.Context, user User, googleSubject string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	profile, err := attributevalue.MarshalMap(userItem{
		PK:          userPK(user.ID),
		SK:          "PROFILE",
		UserID:      user.ID,
		Email:       user.Email,
		Name:        user.Name,
		CreatedAt:   now,
		LastLoginAt: now,
	})
	if err != nil {
		return err
	}
	link, err := attributevalue.MarshalMap(linkItem{
		PK:     googleLinkPK(googleSubject),
		SK:     "LINK",
		UserID: user.ID,
	})
	if err != nil {
		return err
	}
	balance, err := attributevalue.MarshalMap(balanceItem{
		PK:        userPK(user.ID),
		SK:        "BALANCE",
		Remaining: SignupErases,
		UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	_, err = d.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           aws.String(d.table),
					Item:                profile,
					ConditionExpression: aws.String("attribute_not_exists(pk)"),
				},
			},
			{
				Put: &types.Put{
					TableName:           aws.String(d.table),
					Item:                link,
					ConditionExpression: aws.String("attribute_not_exists(pk)"),
				},
			},
			{
				Put: &types.Put{
					TableName:           aws.String(d.table),
					Item:                balance,
					ConditionExpression: aws.String("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
				},
			},
		},
	})
	return err
}

func (d *DynamoDirectory) TouchLogin(ctx context.Context, userID string) error {
	_, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: userPK(userID)},
			"sk": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression: aws.String("SET lastLoginAt = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	return err
}

func (d *DynamoDirectory) Get(ctx context.Context, userID string) (*User, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: userPK(userID)},
			"sk": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}
	var item userItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, err
	}
	return &User{ID: item.UserID, Email: item.Email, Name: item.Name}, nil
}

func (d *DynamoDirectory) Balance(ctx context.Context, userID string) (int, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key:       balanceKey(userID),
	})
	if err != nil {
		return 0, err
	}
	if out.Item == nil {
		return 0, nil
	}
	var item balanceItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return 0, err
	}
	return item.Remaining, nil
}

func (d *DynamoDirectory) Reserve(ctx context.Context, userID string, pages int) error {
	if pages <= 0 {
		return nil
	}
	_, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(d.table),
		Key:                 balanceKey(userID),
		UpdateExpression:    aws.String("SET remaining = remaining - :n, updatedAt = :now"),
		ConditionExpression: aws.String("attribute_exists(pk) AND remaining >= :n"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":n":   &types.AttributeValueMemberN{Value: strconv.Itoa(pages)},
			":now": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	if isConditionFailed(err) {
		return ErrInsufficient
	}
	return err
}

func (d *DynamoDirectory) Refund(ctx context.Context, userID string, pages int) error {
	if pages <= 0 {
		return nil
	}
	_, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(d.table),
		Key:                 balanceKey(userID),
		UpdateExpression:    aws.String("SET remaining = remaining + :n, updatedAt = :now"),
		ConditionExpression: aws.String("attribute_exists(pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":n":   &types.AttributeValueMemberN{Value: strconv.Itoa(pages)},
			":now": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	return err
}

func balanceKey(userID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: userPK(userID)},
		"sk": &types.AttributeValueMemberS{Value: "BALANCE"},
	}
}

func isConditionFailed(err error) bool {
	var failed *types.ConditionalCheckFailedException
	return errors.As(err, &failed)
}

func userPK(userID string) string {
	return "USER#" + userID
}

func googleLinkPK(subject string) string {
	return "ID#google#" + subject
}
