package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type CostControlService struct {
	client    *dynamodb.Client
	tableName string
}

type UserSpendRecord struct {
	UserID      string  `dynamodbav:"user_id"`
	Date        string  `dynamodbav:"date"`
	LLMRequests int     `dynamodbav:"llm_requests"`
	LLMCost     float64 `dynamodbav:"llm_cost"`
	DailyLimit  float64 `dynamodbav:"daily_limit"`
	CreatedAt   string  `dynamodbav:"created_at"`
	UpdatedAt   string  `dynamodbav:"updated_at"`
	TTL         int64   `dynamodbav:"ttl"`
}

type CostControlResult struct {
	Allowed     bool    `json:"allowed"`
	Remaining   float64 `json:"remaining"`
	CurrentCost float64 `json:"current_cost"`
	DailyLimit  float64 `json:"daily_limit"`
	Reason      string  `json:"reason,omitempty"`
}

func NewCostControlService() (*CostControlService, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	tableName := "therma-user-spend"
	if envTable := os.Getenv("USER_SPEND_TABLE_NAME"); envTable != "" {
		tableName = envTable
	}

	client := dynamodb.NewFromConfig(cfg)
	return &CostControlService{
		client:    client,
		tableName: tableName,
	}, nil
}

// CheckUserSpendLimit checks if user can make an LLM request within their daily limit
func (s *CostControlService) CheckUserSpendLimit(ctx context.Context, userID string, estimatedCost float64) (*CostControlResult, error) {
	today := time.Now().Format("2006-01-02")
	
	// Get current spend record
	record, err := s.getUserSpendRecord(ctx, userID, today)
	if err != nil {
		return nil, fmt.Errorf("failed to get user spend record: %v", err)
	}

	// If no record exists, create one with default limit
	if record == nil {
		record = &UserSpendRecord{
			UserID:      userID,
			Date:        today,
			LLMRequests: 0,
			LLMCost:     0.0,
			DailyLimit:  s.getDefaultDailyLimit(userID),
			CreatedAt:   time.Now().Format(time.RFC3339),
			UpdatedAt:   time.Now().Format(time.RFC3339),
		}
	}

	// Check if adding this cost would exceed the limit
	newTotalCost := record.LLMCost + estimatedCost
	
	result := &CostControlResult{
		CurrentCost: record.LLMCost,
		DailyLimit:  record.DailyLimit,
		Remaining:   record.DailyLimit - record.LLMCost,
	}

	if newTotalCost > record.DailyLimit {
		result.Allowed = false
		result.Reason = fmt.Sprintf("Daily limit exceeded. Current: $%.4f, Request: $%.4f, Limit: $%.4f", 
			record.LLMCost, estimatedCost, record.DailyLimit)
		return result, nil
	}

	result.Allowed = true
	return result, nil
}

// RecordLLMRequest records an LLM request and its cost
func (s *CostControlService) RecordLLMRequest(ctx context.Context, userID string, cost float64) error {
	today := time.Now().Format("2006-01-02")
	
	// Get current record
	record, err := s.getUserSpendRecord(ctx, userID, today)
	if err != nil {
		return fmt.Errorf("failed to get user spend record: %v", err)
	}

	// If no record exists, create one
	if record == nil {
		record = &UserSpendRecord{
			UserID:      userID,
			Date:        today,
			LLMRequests: 0,
			LLMCost:     0.0,
			DailyLimit:  s.getDefaultDailyLimit(userID),
			CreatedAt:   time.Now().Format(time.RFC3339),
			UpdatedAt:   time.Now().Format(time.RFC3339),
		}
	}

	// Update the record
	record.LLMRequests++
	record.LLMCost += cost
	record.UpdatedAt = time.Now().Format(time.RFC3339)
	record.TTL = time.Now().Add(7 * 24 * time.Hour).Unix() // Keep records for 7 days

	// Save to DynamoDB
	err = s.saveUserSpendRecord(ctx, record)
	if err != nil {
		return fmt.Errorf("failed to save user spend record: %v", err)
	}

	return nil
}

// GetUserSpendSummary returns the user's current spend summary
func (s *CostControlService) GetUserSpendSummary(ctx context.Context, userID string) (*UserSpendRecord, error) {
	today := time.Now().Format("2006-01-02")
	return s.getUserSpendRecord(ctx, userID, today)
}

// getUserSpendRecord retrieves a user's spend record for a specific date
func (s *CostControlService) getUserSpendRecord(ctx context.Context, userID, date string) (*UserSpendRecord, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"user_id": &types.AttributeValueMemberS{Value: userID},
			"date":    &types.AttributeValueMemberS{Value: date},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %v", err)
	}

	if result.Item == nil {
		return nil, nil // No record found
	}

	var record UserSpendRecord
	err = attributevalue.UnmarshalMap(result.Item, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %v", err)
	}

	return &record, nil
}

// saveUserSpendRecord saves a user's spend record
func (s *CostControlService) saveUserSpendRecord(ctx context.Context, record *UserSpendRecord) error {
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put item: %v", err)
	}

	return nil
}

// getDefaultDailyLimit returns the default daily limit for a user
func (s *CostControlService) getDefaultDailyLimit(userID string) float64 {
	// In a real implementation, this could be based on user tier, subscription, etc.
	// For now, return a reasonable default
	return 5.0 // $5.00 per day
}

// EstimateLLMCost estimates the cost of an LLM request based on input/output tokens
func EstimateLLMCost(inputTokens, outputTokens int, model string) float64 {
	// Cost per 1K tokens for different Bedrock models (as of 2024)
	costs := map[string]struct {
		input  float64
		output float64
	}{
		"anthropic.claude-3-sonnet-20240229-v1:0": {
			input:  0.003,  // $3.00 per 1M input tokens
			output: 0.015,  // $15.00 per 1M output tokens
		},
		"anthropic.claude-3-haiku-20240307-v1:0": {
			input:  0.00025, // $0.25 per 1M input tokens
			output: 0.00125, // $1.25 per 1M output tokens
		},
		"anthropic.claude-3-opus-20240229-v1:0": {
			input:  0.015,   // $15.00 per 1M input tokens
			output: 0.075,   // $75.00 per 1M output tokens
		},
	}

	modelCosts, exists := costs[model]
	if !exists {
		// Default to Claude-3-Sonnet pricing
		modelCosts = costs["anthropic.claude-3-sonnet-20240229-v1:0"]
	}

	inputCost := (float64(inputTokens) / 1000.0) * modelCosts.input
	outputCost := (float64(outputTokens) / 1000.0) * modelCosts.output

	return inputCost + outputCost
}

// GetGracefulDegradationOptions returns options for graceful degradation when limits are hit
func GetGracefulDegradationOptions() []string {
	return []string{
		"Use cached/summarized content",
		"Reduce response length",
		"Use cheaper model (Claude Haiku instead of Sonnet)",
		"Batch multiple requests",
		"Return partial results",
		"Suggest user upgrade plan",
	}
}
