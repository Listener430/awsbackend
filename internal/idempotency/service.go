package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type IdempotencyService struct {
	client    *dynamodb.Client
	tableName string
}

type IdempotencyRecord struct {
	Key         string    `dynamodbav:"key"`
	UserID      string    `dynamodbav:"user_id"`
	RequestHash string    `dynamodbav:"request_hash"`
	Response    string    `dynamodbav:"response"`
	Status      string    `dynamodbav:"status"`
	CreatedAt   time.Time `dynamodbav:"created_at"`
	ExpiresAt   time.Time `dynamodbav:"expires_at"`
	TTL         int64     `dynamodbav:"ttl"`
}

func NewIdempotencyService() (*IdempotencyService, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	tableName := "therma-idempotency"
	if envTable := os.Getenv("IDEMPOTENCY_TABLE_NAME"); envTable != "" {
		tableName = envTable
	}

	client := dynamodb.NewFromConfig(cfg)
	return &IdempotencyService{
		client:    client,
		tableName: tableName,
	}, nil
}

// GenerateIdempotencyKey creates a unique key for the request
func (s *IdempotencyService) GenerateIdempotencyKey(userID, endpoint, requestBody string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", userID, endpoint, requestBody)))
	return hex.EncodeToString(hash[:])
}

// GenerateRequestHash creates a hash of the request for comparison
func (s *IdempotencyService) GenerateRequestHash(requestBody string) string {
	hash := sha256.Sum256([]byte(requestBody))
	return hex.EncodeToString(hash[:])
}

// CheckIdempotency checks if a request with the same idempotency key already exists
func (s *IdempotencyService) CheckIdempotency(ctx context.Context, key string) (*IdempotencyRecord, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"key": &types.AttributeValueMemberS{Value: key},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to check idempotency: %v", err)
	}

	if result.Item == nil {
		return nil, nil // No existing record
	}

	var record IdempotencyRecord
	err = attributevalue.UnmarshalMap(result.Item, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal idempotency record: %v", err)
	}

	// Check if record has expired
	if time.Now().After(record.ExpiresAt) {
		// Record expired, delete it
		s.DeleteIdempotencyRecord(ctx, key)
		return nil, nil
	}

	return &record, nil
}

// StoreIdempotencyRecord stores a new idempotency record
func (s *IdempotencyService) StoreIdempotencyRecord(ctx context.Context, record *IdempotencyRecord) error {
	// Set TTL for automatic cleanup (24 hours from now)
	record.TTL = time.Now().Add(24 * time.Hour).Unix()

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal idempotency record: %v", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
		ConditionExpression: aws.String("attribute_not_exists(#key)"),
		ExpressionAttributeNames: map[string]string{
			"#key": "key",
		},
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		// Check if it's a conditional check failed error (record already exists)
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			return fmt.Errorf("idempotency key already exists")
		}
		return fmt.Errorf("failed to store idempotency record: %v", err)
	}

	return nil
}

// UpdateIdempotencyRecord updates an existing idempotency record with response
func (s *IdempotencyService) UpdateIdempotencyRecord(ctx context.Context, key, response, status string) error {
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"key": &types.AttributeValueMemberS{Value: key},
		},
		UpdateExpression: aws.String("SET #response = :response, #status = :status, #updated_at = :updated_at"),
		ExpressionAttributeNames: map[string]string{
			"#response":    "response",
			"#status":      "status",
			"#updated_at":  "updated_at",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":response":   &types.AttributeValueMemberS{Value: response},
			":status":     &types.AttributeValueMemberS{Value: status},
			":updated_at": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}

	_, err := s.client.UpdateItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update idempotency record: %v", err)
	}

	return nil
}

// DeleteIdempotencyRecord deletes an idempotency record
func (s *IdempotencyService) DeleteIdempotencyRecord(ctx context.Context, key string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"key": &types.AttributeValueMemberS{Value: key},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete idempotency record: %v", err)
	}

	return nil
}

// ProcessIdempotentRequest handles the complete idempotency flow
func (s *IdempotencyService) ProcessIdempotentRequest(
	ctx context.Context,
	userID, endpoint, requestBody string,
	handler func() (interface{}, error),
) (interface{}, error) {
	// Generate idempotency key and request hash
	key := s.GenerateIdempotencyKey(userID, endpoint, requestBody)
	requestHash := s.GenerateRequestHash(requestBody)

	// Check if request already exists
	existingRecord, err := s.CheckIdempotency(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to check idempotency: %v", err)
	}

	// If record exists and request hash matches, return cached response
	if existingRecord != nil && existingRecord.RequestHash == requestHash {
		if existingRecord.Status == "completed" {
			var response interface{}
			err := json.Unmarshal([]byte(existingRecord.Response), &response)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal cached response: %v", err)
			}
			return response, nil
		} else if existingRecord.Status == "pending" {
			return nil, fmt.Errorf("request is already being processed")
		}
	}

	// If record exists but request hash doesn't match, it's a duplicate key with different content
	if existingRecord != nil && existingRecord.RequestHash != requestHash {
		return nil, fmt.Errorf("idempotency key conflict: same key used for different request")
	}

	// Create new idempotency record
	record := &IdempotencyRecord{
		Key:         key,
		UserID:      userID,
		RequestHash: requestHash,
		Status:      "pending",
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}

	err = s.StoreIdempotencyRecord(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("failed to store idempotency record: %v", err)
	}

	// Execute the actual handler
	response, err := handler()
	if err != nil {
		// Update record with error status
		s.UpdateIdempotencyRecord(ctx, key, fmt.Sprintf("error: %v", err), "failed")
		return nil, err
	}

	// Marshal response for storage
	responseJSON, err := json.Marshal(response)
	if err != nil {
		s.UpdateIdempotencyRecord(ctx, key, "error: failed to marshal response", "failed")
		return nil, fmt.Errorf("failed to marshal response: %v", err)
	}

	// Update record with successful response
	err = s.UpdateIdempotencyRecord(ctx, key, string(responseJSON), "completed")
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Warning: failed to update idempotency record: %v\n", err)
	}

	return response, nil
}
