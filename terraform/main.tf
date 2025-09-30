provider "aws" {
  region = "eu-north-1"
}

# VPC and Security Group for RDS
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name = "therma-vpc"
  }
}

# Create subnets for RDS
resource "aws_subnet" "private_1" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "eu-north-1a"

  tags = {
    Name = "private-subnet-1"
  }
}

resource "aws_subnet" "private_2" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.2.0/24"
  availability_zone = "eu-north-1b"

  tags = {
    Name = "private-subnet-2"
  }
}

# Create DB subnet group
resource "aws_db_subnet_group" "postgres" {
  name       = "therma-db-subnet-group"
  subnet_ids = [aws_subnet.private_1.id, aws_subnet.private_2.id]

  tags = {
    Name = "therma-db-subnet-group"
  }
}

resource "aws_security_group" "rds" {
  name        = "therma-rds-sg"
  description = "Security group for RDS instance"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# RDS Instance
resource "aws_db_instance" "postgres" {
  identifier           = "therma-db"
  engine              = "postgres"
  engine_version      = "14.18"
  instance_class      = "db.t3.micro"
  allocated_storage   = 20
  storage_type        = "gp2"
  username            = "postgres"
  password            = var.db_password
  skip_final_snapshot = true

  vpc_security_group_ids = [aws_security_group.rds.id]
  db_subnet_group_name   = aws_db_subnet_group.postgres.name
  publicly_accessible    = true

  tags = {
    Name = "therma-db"
  }

  depends_on = [
    aws_internet_gateway.gw,
    aws_route_table_association.a,
    aws_route_table_association.b
  ]
}

# IAM Role for Lambda functions
resource "aws_iam_role" "lambda_role" {
  name = "therma-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

# IAM Policy for Lambda functions
resource "aws_iam_role_policy" "lambda_policy" {
  name = "therma-lambda-policy"
  role = aws_iam_role.lambda_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:*:*:*"
      },
      {
        Effect = "Allow"
        Action = [
          "states:StartExecution"
        ]
        Resource = aws_sfn_state_machine.dating_app.arn
      }
    ]
  })
}

# Lambda Functions
resource "aws_lambda_function" "journal_entry" {
  filename         = "../bin/journal-entry.zip"
  function_name    = "journal-entry"
  role            = aws_iam_role.lambda_role.arn
  handler         = "bootstrap"
  runtime         = "provided.al2"
  timeout         = 30

  environment {
    variables = {
      DATABASE_URL = "postgres://postgres:${var.db_password}@${aws_db_instance.postgres.endpoint}/postgres?sslmode=disable"
      JWT_SECRET   = var.jwt_secret
      JWT_ISSUER   = "therma-api"
      JWT_TTL      = "1h"
      KMS_KEY_ID   = aws_kms_key.phi_encryption_key.key_id
    }
  }
}

# Step Functions State Machine
resource "aws_iam_role" "step_functions_role" {
  name = "therma-step-functions-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "states.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "step_functions_policy" {
  name = "therma-step-functions-policy"
  role = aws_iam_role.step_functions_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "lambda:InvokeFunction"
        ]
        Resource = [
          aws_lambda_function.journal_entry.arn
        ]
      }
    ]
  })
}

resource "aws_sfn_state_machine" "journal_processing" {
  name     = "therma-journal-workflow"
  role_arn = aws_iam_role.step_functions_role.arn
  definition = templatefile("${path.module}/step-functions/journal-workflow.json", {
    journal_entry_arn = aws_lambda_function.journal_entry.arn
  })
}

resource "aws_internet_gateway" "gw" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "therma-gw"
  }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.gw.id
  }

  tags = {
    Name = "public-route-table"
  }
}

resource "aws_route_table_association" "a" {
  subnet_id      = aws_subnet.private_1.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "b" {
  subnet_id      = aws_subnet.private_2.id
  route_table_id = aws_route_table.public.id
}

# API Gateway REST API
resource "aws_api_gateway_rest_api" "therma_api" {
  name = "therma-api"
}

resource "aws_api_gateway_resource" "journal_entries" {
  rest_api_id = aws_api_gateway_rest_api.therma_api.id
  parent_id   = aws_api_gateway_rest_api.therma_api.root_resource_id
  path_part   = "journal-entries"
}

resource "aws_api_gateway_method" "journal_entries_post" {
  rest_api_id   = aws_api_gateway_rest_api.therma_api.id
  resource_id   = aws_api_gateway_resource.journal_entries.id
  http_method   = "POST"
  authorization = "NONE"
}

resource "aws_api_gateway_integration" "journal_entries_integration" {
  rest_api_id             = aws_api_gateway_rest_api.therma_api.id
  resource_id             = aws_api_gateway_resource.journal_entries.id
  http_method             = aws_api_gateway_method.journal_entries_post.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.journal_entry.invoke_arn
}

resource "aws_lambda_permission" "apigw_lambda" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.journal_entry.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_rest_api.therma_api.execution_arn}/*/*"
}

resource "aws_api_gateway_deployment" "therma_api" {
  rest_api_id = aws_api_gateway_rest_api.therma_api.id
  stage_name  = "prod"
  depends_on  = [aws_api_gateway_integration.journal_entries_integration]
}

# KMS Key for PHI Encryption
resource "aws_kms_key" "phi_encryption_key" {
  description             = "KMS key for PHI encryption in Therma backend"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = {
    Name        = "therma-phi-encryption-key"
    Environment = "dev"
    Purpose     = "PHI-Encryption"
    Compliance  = "HIPAA"
  }
}

# SQS Queue for Journal Processing
resource "aws_sqs_queue" "journal_processing_queue" {
  name                       = "therma-journal-processing"
  delay_seconds              = 0
  max_message_size           = 262144
  message_retention_seconds  = 1209600
  receive_wait_time_seconds  = 0
  visibility_timeout_seconds = 300

  kms_master_key_id = aws_kms_key.phi_encryption_key.arn
  kms_data_key_reuse_period_seconds = 300

  tags = {
    Name        = "therma-journal-processing"
    Environment = "dev"
    Compliance  = "HIPAA"
  }
}

# Dead Letter Queue
resource "aws_sqs_queue" "journal_processing_dlq" {
  name = "therma-journal-processing-dlq"

  kms_master_key_id = aws_kms_key.phi_encryption_key.arn
  kms_data_key_reuse_period_seconds = 300

  tags = {
    Name        = "therma-journal-processing-dlq"
    Environment = "dev"
    Compliance  = "HIPAA"
  }
}

# S3 Bucket for Audit Logs
resource "aws_s3_bucket" "audit_logs" {
  bucket = "therma-audit-logs-${random_string.bucket_suffix.result}"

  tags = {
    Name        = "therma-audit-logs"
    Environment = "dev"
    Compliance  = "HIPAA"
  }
}

resource "aws_s3_bucket_versioning" "audit_logs" {
  bucket = aws_s3_bucket.audit_logs.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "audit_logs" {
  bucket = aws_s3_bucket.audit_logs.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.phi_encryption_key.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_object_lock_configuration" "audit_logs" {
  bucket = aws_s3_bucket.audit_logs.id

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = 2555
    }
  }
}

# Random string for unique bucket names
resource "random_string" "bucket_suffix" {
  length  = 8
  special = false
  upper   = false
} 