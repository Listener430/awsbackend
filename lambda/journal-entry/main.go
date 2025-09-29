package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awsbackend/internal/auth"
	"github.com/awsbackend/internal/encryption"
	"github.com/awsbackend/internal/idempotency"
	"github.com/awsbackend/internal/llm"
	"github.com/awsbackend/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type JournalEntryRequest struct {
	Content   string   `json:"content"`
	Mood      string   `json:"mood"`
	Tags      []string `json:"tags"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type JournalEntryResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	Mood      string    `json:"mood"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Encrypted bool      `json:"encrypted"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract user ID from JWT token
	userID, err := extractUserIDFromRequest(request)
	if err != nil {
		return createErrorResponse(401, "UNAUTHORIZED", "Invalid or missing authentication token", err.Error()), nil
	}

	// Parse request body
	var req JournalEntryRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return createErrorResponse(400, "INVALID_REQUEST", "Invalid JSON in request body", err.Error()), nil
	}

	// Validate required fields
	if req.Content == "" {
		return createErrorResponse(400, "VALIDATION_ERROR", "Content is required", ""), nil
	}

	// Initialize services
	idempotencyService, err := idempotency.NewIdempotencyService()
	if err != nil {
		return createErrorResponse(500, "SERVICE_ERROR", "Failed to initialize idempotency service", err.Error()), nil
	}

	kmsService, err := encryption.NewKMSClient()
	if err != nil {
		return createErrorResponse(500, "SERVICE_ERROR", "Failed to initialize encryption service", err.Error()), nil
	}

	costControlService, err := llm.NewCostControlService()
	if err != nil {
		return createErrorResponse(500, "SERVICE_ERROR", "Failed to initialize cost control service", err.Error()), nil
	}

	// Process request with idempotency
	response, err := idempotencyService.ProcessIdempotentRequest(
		ctx,
		userID,
		"POST /journal-entries",
		request.Body,
		func() (interface{}, error) {
			return processJournalEntry(ctx, userID, req, kmsService, costControlService)
		},
	)

	if err != nil {
		return createErrorResponse(500, "PROCESSING_ERROR", "Failed to process journal entry", err.Error()), nil
	}

	// Convert response to JSON
	responseBody, err := json.Marshal(response)
	if err != nil {
		return createErrorResponse(500, "SERIALIZATION_ERROR", "Failed to serialize response", err.Error()), nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 201,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(responseBody),
	}, nil
}

func processJournalEntry(
	ctx context.Context,
	userID string,
	req JournalEntryRequest,
	kmsService *encryption.KMSClient,
	costControlService *llm.CostControlService,
) (*JournalEntryResponse, error) {
	// Check LLM cost limits before processing
	estimatedCost := llm.EstimateLLMCost(len(req.Content), 100, "anthropic.claude-3-sonnet-20240229-v1:0")
	costCheck, err := costControlService.CheckUserSpendLimit(ctx, userID, estimatedCost)
	if err != nil {
		return nil, fmt.Errorf("failed to check cost limits: %v", err)
	}

	if !costCheck.Allowed {
		// Implement graceful degradation
		return handleCostLimitExceeded(req, costCheck)
	}

	// Encrypt PHI data
	encryptedContent, err := kmsService.EncryptPHI(ctx, req.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt content: %v", err)
	}

	encryptedMood, err := kmsService.EncryptPHI(ctx, req.Mood)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt mood: %v", err)
	}

	encryptedTags, err := kmsService.EncryptPHIArray(ctx, req.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt tags: %v", err)
	}

	// Create journal entry (in a real implementation, this would save to database)
	entry := &JournalEntryResponse{
		ID:        generateID(),
		UserID:    userID,
		Content:   encryptedContent,
		Mood:      encryptedMood,
		Tags:      encryptedTags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Encrypted: true,
	}

	// Record the LLM cost (even though we didn't use LLM in this example)
	err = costControlService.RecordLLMRequest(ctx, userID, estimatedCost)
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Warning: failed to record LLM cost: %v\n", err)
	}

	return entry, nil
}

func handleCostLimitExceeded(req JournalEntryRequest, costCheck *llm.CostControlResult) (*JournalEntryResponse, error) {
	// Graceful degradation: return a basic response without LLM processing
	entry := &JournalEntryResponse{
		ID:        generateID(),
		UserID:    "user_id", // This would be the actual user ID
		Content:   req.Content, // Store unencrypted for now (in real implementation, still encrypt)
		Mood:      req.Mood,
		Tags:      req.Tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Encrypted: false,
	}

	// In a real implementation, you might:
	// 1. Use a cheaper model
	// 2. Return cached/summarized content
	// 3. Suggest user upgrade
	// 4. Queue for later processing

	return entry, nil
}

func extractUserIDFromRequest(request events.APIGatewayProxyRequest) (string, error) {
	// Extract JWT token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	// Remove "Bearer " prefix
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		authHeader = authHeader[7:]
	}

	// Validate token and extract user ID
	claims, err := auth.ValidateToken(authHeader)
	if err != nil {
		return "", fmt.Errorf("invalid token: %v", err)
	}

	return claims.UserID, nil
}

func createErrorResponse(statusCode int, code, message, details string) events.APIGatewayProxyResponse {
	errorResp := ErrorResponse{
		Error:   message,
		Code:    code,
		Details: details,
	}

	body, _ := json.Marshal(errorResp)
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

func generateID() string {
	return fmt.Sprintf("entry_%d", time.Now().UnixNano())
}

func main() {
	lambda.Start(handler)
}
