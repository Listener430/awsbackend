# HIPAA-Compliant Infrastructure for Therma Backend
# This demonstrates the PHI boundary and security controls

# KMS Key for PHI Encryption
resource "aws_kms_key" "phi_encryption_key" {
  description             = "KMS key for PHI encryption in Therma backend"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = {
    Name        = "therma-phi-encryption-key"
    Environment = "production"
    Purpose     = "PHI-Encryption"
    Compliance  = "HIPAA"
  }
}

resource "aws_kms_alias" "phi_encryption_key_alias" {
  name          = "alias/therma-phi-encryption"
  target_key_id = aws_kms_key.phi_encryption_key.key_id
}

# KMS Key Policy for HIPAA compliance
resource "aws_kms_key_policy" "phi_encryption_key_policy" {
  key_id = aws_kms_key.phi_encryption_key.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "Enable IAM User Permissions"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"
        }
        Action   = "kms:*"
        Resource = "*"
      },
      {
        Sid    = "Allow Lambda Functions"
        Effect = "Allow"
        Principal = {
          AWS = aws_iam_role.lambda_role.arn
        }
        Action = [
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:ReEncrypt*",
          "kms:GenerateDataKey*",
          "kms:DescribeKey"
        ]
        Resource = "*"
        Condition = {
          StringEquals = {
            "kms:EncryptionContext:Service" = "Therma-Backend"
            "kms:EncryptionContext:Purpose" = "PHI-Encryption"
          }
        }
      },
      {
        Sid    = "Allow CloudWatch Logs"
        Effect = "Allow"
        Principal = {
          Service = "logs.amazonaws.com"
        }
        Action = [
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:ReEncrypt*",
          "kms:GenerateDataKey*",
          "kms:DescribeKey"
        ]
        Resource = "*"
        Condition = {
          ArnEquals = {
            "kms:EncryptionContext:aws:logs:arn" = "arn:aws:logs:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:*"
          }
        }
      }
    ]
  })
}

# DynamoDB Tables for Idempotency and Cost Control
resource "aws_dynamodb_table" "idempotency_table" {
  name           = "therma-idempotency"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "key"

  attribute {
    name = "key"
    type = "S"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }

  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled     = true
    kms_key_id  = aws_kms_key.phi_encryption_key.arn
  }

  tags = {
    Name        = "therma-idempotency"
    Environment = "production"
    Compliance  = "HIPAA"
  }
}

resource "aws_dynamodb_table" "user_spend_table" {
  name           = "therma-user-spend"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "user_id"
  range_key      = "date"

  attribute {
    name = "user_id"
    type = "S"
  }

  attribute {
    name = "date"
    type = "S"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }

  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled     = true
    kms_key_id  = aws_kms_key.phi_encryption_key.arn
  }

  tags = {
    Name        = "therma-user-spend"
    Environment = "production"
    Compliance  = "HIPAA"
  }
}

# S3 Bucket for Audit Logs with Object Lock
resource "aws_s3_bucket" "audit_logs" {
  bucket = "therma-audit-logs-${random_string.bucket_suffix.result}"

  tags = {
    Name        = "therma-audit-logs"
    Environment = "production"
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
      days = 2555 # 7 years for HIPAA compliance
    }
  }
}

resource "aws_s3_bucket_public_access_block" "audit_logs" {
  bucket = aws_s3_bucket.audit_logs.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# CloudTrail for Audit Logging
resource "aws_cloudtrail" "therma_audit_trail" {
  name                          = "therma-audit-trail"
  s3_bucket_name               = aws_s3_bucket.audit_logs.id
  include_global_service_events = true
  is_multi_region_trail        = true
  enable_logging               = true

  event_selector {
    read_write_type                 = "All"
    include_management_events       = true
    data_resource {
      type   = "AWS::S3::Object"
      values = ["${aws_s3_bucket.audit_logs.arn}/*"]
    }
  }

  event_selector {
    read_write_type                 = "All"
    include_management_events       = true
    data_resource {
      type   = "AWS::DynamoDB::Table"
      values = [
        aws_dynamodb_table.idempotency_table.arn,
        aws_dynamodb_table.user_spend_table.arn
      ]
    }
  }

  depends_on = [aws_s3_bucket_policy.audit_logs]

  tags = {
    Name        = "therma-audit-trail"
    Environment = "production"
    Compliance  = "HIPAA"
  }
}

resource "aws_s3_bucket_policy" "audit_logs" {
  bucket = aws_s3_bucket.audit_logs.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AWSCloudTrailAclCheck"
        Effect = "Allow"
        Principal = {
          Service = "cloudtrail.amazonaws.com"
        }
        Action   = "s3:GetBucketAcl"
        Resource = aws_s3_bucket.audit_logs.arn
        Condition = {
          StringEquals = {
            "AWS:SourceArn" = "arn:aws:cloudtrail:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:trail/therma-audit-trail"
          }
        }
      },
      {
        Sid    = "AWSCloudTrailWrite"
        Effect = "Allow"
        Principal = {
          Service = "cloudtrail.amazonaws.com"
        }
        Action   = "s3:PutObject"
        Resource = "${aws_s3_bucket.audit_logs.arn}/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl"        = "bucket-owner-full-control"
            "AWS:SourceArn"       = "arn:aws:cloudtrail:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:trail/therma-audit-trail"
          }
        }
      }
    ]
  })
}

# Enhanced IAM Role for Lambda with HIPAA-compliant permissions
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

  tags = {
    Name        = "therma-lambda-role"
    Environment = "production"
    Compliance  = "HIPAA"
  }
}

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
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:ReEncrypt*",
          "kms:GenerateDataKey*",
          "kms:DescribeKey"
        ]
        Resource = aws_kms_key.phi_encryption_key.arn
        Condition = {
          StringEquals = {
            "kms:EncryptionContext:Service" = "Therma-Backend"
            "kms:EncryptionContext:Purpose" = "PHI-Encryption"
          }
        }
      },
      {
        Effect = "Allow"
        Action = [
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "dynamodb:UpdateItem",
          "dynamodb:DeleteItem",
          "dynamodb:Query",
          "dynamodb:Scan"
        ]
        Resource = [
          aws_dynamodb_table.idempotency_table.arn,
          aws_dynamodb_table.user_spend_table.arn
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "bedrock:InvokeModel",
          "bedrock:InvokeModelWithResponseStream"
        ]
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "rds:DescribeDBInstances",
          "rds:Connect"
        ]
        Resource = "*"
      }
    ]
  })
}

# SQS Queue for async processing
resource "aws_sqs_queue" "journal_processing_queue" {
  name                       = "therma-journal-processing"
  delay_seconds              = 0
  max_message_size           = 262144
  message_retention_seconds  = 1209600 # 14 days
  receive_wait_time_seconds  = 0
  visibility_timeout_seconds = 300

  kms_master_key_id = aws_kms_key.phi_encryption_key.arn
  kms_data_key_reuse_period_seconds = 300

  tags = {
    Name        = "therma-journal-processing"
    Environment = "production"
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
    Environment = "production"
    Compliance  = "HIPAA"
  }
}

# Step Functions State Machine for Journal Processing
resource "aws_sfn_state_machine" "journal_processing" {
  name     = "therma-journal-processing"
  role_arn = aws_iam_role.step_functions_role.arn

  definition = jsonencode({
    Comment = "Therma Journal Processing Workflow"
    StartAt = "ValidateRequest"
    States = {
      ValidateRequest = {
        Type     = "Task"
        Resource = aws_lambda_function.validate_journal_entry.arn
        Next     = "CheckCostLimits"
        Retry = [
          {
            ErrorEquals     = ["States.ALL"]
            IntervalSeconds = 2
            MaxAttempts     = 3
            BackoffRate     = 2.0
          }
        ]
      }
      CheckCostLimits = {
        Type     = "Task"
        Resource = aws_lambda_function.check_cost_limits.arn
        Next     = "ProcessJournalEntry"
        Catch = [
          {
            ErrorEquals = ["CostLimitExceeded"]
            Next        = "GracefulDegradation"
            ResultPath  = "$.error"
          }
        ]
      }
      ProcessJournalEntry = {
        Type     = "Task"
        Resource = aws_lambda_function.process_journal_entry.arn
        Next     = "SendToLLM"
      }
      SendToLLM = {
        Type     = "Task"
        Resource = aws_lambda_function.llm_processing.arn
        Next     = "Complete"
        Retry = [
          {
            ErrorEquals     = ["States.ALL"]
            IntervalSeconds = 5
            MaxAttempts     = 2
            BackoffRate     = 2.0
          }
        ]
      }
      GracefulDegradation = {
        Type     = "Task"
        Resource = aws_lambda_function.graceful_degradation.arn
        Next     = "Complete"
      }
      Complete = {
        Type = "Succeed"
      }
    }
  })

  tags = {
    Name        = "therma-journal-processing"
    Environment = "production"
    Compliance  = "HIPAA"
  }
}

# Data sources
data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

# Random string for unique bucket names
resource "random_string" "bucket_suffix" {
  length  = 8
  special = false
  upper   = false
}
