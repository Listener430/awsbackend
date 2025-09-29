# AWS Backend - HIPAA Serverless

Serverless backend on AWS for journaling/mood tracking. Has HIPAA compliance, idempotency, and LLM cost controls.

## Stack
- API Gateway for REST endpoints
- Lambda functions in Go
- DynamoDB for idempotency & cost tracking
- KMS for PHI encryption
- SQS for async processing
- Step Functions for workflows
- CloudTrail for audit logs
- S3 with Object Lock for immutable logs

## Key Features
- KMS envelope encryption for PHI
- Idempotency with DynamoDB
- LLM cost limits per user
- Graceful degradation when limits hit
- Audit logging everywhere

## Setup
1. Set AWS creds
2. Set env vars: JWT_SECRET, KMS_KEY_ID, DATABASE_URL
3. Deploy: `terraform init && terraform apply`

## APIs
- POST /register - user signup
- POST /journal-entries - create entry
- GET /mood-checkins - get mood data

All PHI encrypted at rest, audit logs in S3, least privilege IAM.