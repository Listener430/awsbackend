package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awsbackend/internal/auth"
	"github.com/awsbackend/internal/db"
)

type NotificationRequest struct {
	MatchID string `json:"match_id"`
}

type NotificationResponse struct {
	Success bool `json:"success"`
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Validate JWT token
	claims, err := auth.ValidateToken(request.Headers["Authorization"])
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 401,
			Body:       "Unauthorized",
		}, nil
	}

	var req NotificationRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       fmt.Sprintf("Invalid request: %v", err),
		}, nil
	}

	// Verify that the user is part of the match
	var user1ID, user2ID string
	err = db.DB.QueryRow(
		"SELECT user1_id, user2_id FROM matches WHERE id = $1",
		req.MatchID,
	).Scan(&user1ID, &user2ID)

	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 404,
			Body:       "Match not found",
		}, nil
	}

	if claims.UserID != user1ID && claims.UserID != user2ID {
		return events.APIGatewayProxyResponse{
			StatusCode: 403,
			Body:       "Not authorized to send notification for this match",
		}, nil
	}

	// In a real application, this would send actual notifications
	// For this example, we'll just simulate success
	response := NotificationResponse{
		Success: true,
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Error creating response",
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(responseBody),
	}, nil
}

func init() {
	if err := db.InitDB(); err != nil {
		fmt.Printf("Error initializing database: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	lambda.Start(handler)
}
