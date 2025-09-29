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
	"github.com/awsbackend/internal/models"
)

type CreateProfileRequest struct {
	Interests   []string `json:"interests"`
	Preferences []string `json:"preferences"`
}

type CreateProfileResponse struct {
	ProfileID string `json:"profile_id"`
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

	var req CreateProfileRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       fmt.Sprintf("Invalid request: %v", err),
		}, nil
	}

	// Create profile in database
	profile := models.Profile{
		UserID:      claims.UserID,
		Interests:   req.Interests,
		Preferences: req.Preferences,
	}

	var profileID string
	err = db.DB.QueryRow(
		"INSERT INTO profiles (user_id, interests, preferences) VALUES ($1, $2, $3) RETURNING id",
		profile.UserID, profile.Interests, profile.Preferences,
	).Scan(&profileID)

	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("Error creating profile: %v", err),
		}, nil
	}

	response := CreateProfileResponse{
		ProfileID: profileID,
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
