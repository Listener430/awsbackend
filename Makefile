.PHONY: build clean deploy

# Build all Lambda functions
build:
	@echo "Building Lambda functions..."
	GOOS=linux GOARCH=amd64 go build -o bin/bootstrap ./lambda/register-user
	GOOS=linux GOARCH=amd64 go build -o bin/create-profile ./lambda/create-profile
	GOOS=linux GOARCH=amd64 go build -o bin/match-user ./lambda/match-user
	GOOS=linux GOARCH=amd64 go build -o bin/send-notification ./lambda/send-notification

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf ./bin

# Deploy using Terraform
deploy:
	@echo "Deploying infrastructure..."
	terraform init
	terraform apply -auto-approve

# Run locally
run-local:
	@echo "Running locally..."
	go run cmd/main.go 