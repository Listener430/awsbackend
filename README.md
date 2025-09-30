# Therma Backend - Starter Repo

HIPAA-compliant serverless backend starter for journaling and mood tracking. Ready for pilot implementation.

## Architecture Overview
- API Gateway with Cognito auth
- Go Lambda functions
- Aurora PostgreSQL with migrations
- KMS envelope encryption for PHI
- SQS + Step Functions for event processing
- S3 Object Lock for immutable audit logs
- OpenTelemetry/X-Ray tracing

## Pilot Scope
- Auth: Cognito + Apple/Google OIDC
- Journal API: Go Lambdas with KMS encryption
- Event Trail: SQS → Step Functions → S3 audit
- Ops: Terraform, GitHub Actions, one-command deploy

## Getting Started
1. Set AWS credentials
2. Configure environment variables
3. Deploy infrastructure: `terraform init && terraform apply`
4. Build and deploy Lambda functions

## Next Steps
- Add Cognito user pool configuration
- Implement OIDC providers
- Add database migrations
- Set up CI/CD pipeline
- Add OpenTelemetry instrumentation

This is a starter repo - full implementation requires AWS access and additional configuration.