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

type MatchRequest struct {
	TargetUserID string `json:"target_user_id"`
}

type MatchResponse struct {
	MatchID string  `json:"match_id"`
	Score   float64 `json:"score"`
}

func calculateMatchScore(user1Interests, user1Preferences, user2Interests, user2Preferences []string) float64 {
	// Simple matching algorithm based on common interests and preferences
	commonInterests := 0
	commonPreferences := 0

	// Count common interests
	for _, interest1 := range user1Interests {
		for _, interest2 := range user2Interests {
			if interest1 == interest2 {
				commonInterests++
			}
		}
	}

	// Count common preferences
	for _, pref1 := range user1Preferences {
		for _, pref2 := range user2Preferences {
			if pref1 == pref2 {
				commonPreferences++
			}
		}
	}

	// Calculate score (0-100)
	totalInterests := len(user1Interests) + len(user2Interests)
	totalPreferences := len(user1Preferences) + len(user2Preferences)

	if totalInterests == 0 && totalPreferences == 0 {
		return 0
	}

	interestScore := float64(commonInterests*2) / float64(totalInterests) * 50
	preferenceScore := float64(commonPreferences*2) / float64(totalPreferences) * 50

	return interestScore + preferenceScore
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

	var req MatchRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       fmt.Sprintf("Invalid request: %v", err),
		}, nil
	}

	// Get both users' profiles
	var user1Interests, user1Preferences, user2Interests, user2Preferences []string
	err = db.DB.QueryRow(
		"SELECT interests, preferences FROM profiles WHERE user_id = $1",
		claims.UserID,
	).Scan(&user1Interests, &user1Preferences)

	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Error fetching user profile",
		}, nil
	}

	err = db.DB.QueryRow(
		"SELECT interests, preferences FROM profiles WHERE user_id = $1",
		req.TargetUserID,
	).Scan(&user2Interests, &user2Preferences)

	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Error fetching target user profile",
		}, nil
	}

	// Calculate match score
	score := calculateMatchScore(user1Interests, user1Preferences, user2Interests, user2Preferences)

	// Create match record if score is above threshold (e.g., 50)
	if score >= 50 {
		var matchID string
		err = db.DB.QueryRow(
			"INSERT INTO matches (user1_id, user2_id, score) VALUES ($1, $2, $3) RETURNING id",
			claims.UserID, req.TargetUserID, score,
		).Scan(&matchID)

		if err != nil {
			return events.APIGatewayProxyResponse{
				StatusCode: 500,
				Body:       "Error creating match record",
			}, nil
		}

		response := MatchResponse{
			MatchID: matchID,
			Score:   score,
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

	// No match found
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       `{"message": "No match found"}`,
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
