provider "aws" {
  region = "eu-north-1"
}

# VPC and Security Group for RDS
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name = "dating-app-vpc"
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
  name       = "dating-app-db-subnet-group"
  subnet_ids = [aws_subnet.private_1.id, aws_subnet.private_2.id]

  tags = {
    Name = "dating-app-db-subnet-group"
  }
}

resource "aws_security_group" "rds" {
  name        = "dating-app-rds-sg"
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
  identifier           = "dating-app-db"
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
    Name = "dating-app-db"
  }

  depends_on = [
    aws_internet_gateway.gw,
    aws_route_table_association.a,
    aws_route_table_association.b
  ]
}

# IAM Role for Lambda functions
resource "aws_iam_role" "lambda_role" {
  name = "dating-app-lambda-role"

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
  name = "dating-app-lambda-policy"
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
resource "aws_lambda_function" "register_user" {
  filename         = "../bin/register-user.zip"
  function_name    = "register-user"
  role            = aws_iam_role.lambda_role.arn
  handler         = "bootstrap"
  runtime         = "provided.al2"
  timeout         = 30

  environment {
    variables = {
      DATABASE_URL = "postgres://postgres:${var.db_password}@${aws_db_instance.postgres.endpoint}/postgres?sslmode=disable"
      JWT_SECRET   = var.jwt_secret
      JWT_ISSUER   = "banking-api"
      JWT_TTL      = "1h"
    }
  }
}

resource "aws_lambda_function" "create_profile" {
  filename         = "../bin/create-profile.zip"
  function_name    = "create-profile"
  role            = aws_iam_role.lambda_role.arn
  handler         = "create-profile"
  runtime         = "provided.al2"
  timeout         = 30

  environment {
    variables = {
      DATABASE_URL = aws_db_instance.postgres.endpoint
      JWT_SECRET   = var.jwt_secret
      JWT_ISSUER   = "banking-api"
      JWT_TTL      = "1h"
    }
  }
}

resource "aws_lambda_function" "match_user" {
  filename         = "../bin/match-user.zip"
  function_name    = "match-user"
  role            = aws_iam_role.lambda_role.arn
  handler         = "match-user"
  runtime         = "provided.al2"
  timeout         = 30

  environment {
    variables = {
      DATABASE_URL = aws_db_instance.postgres.endpoint
      JWT_SECRET   = var.jwt_secret
      JWT_ISSUER   = "banking-api"
      JWT_TTL      = "1h"
    }
  }
}

resource "aws_lambda_function" "send_notification" {
  filename         = "../bin/send-notification.zip"
  function_name    = "send-notification"
  role            = aws_iam_role.lambda_role.arn
  handler         = "send-notification"
  runtime         = "provided.al2"
  timeout         = 30

  environment {
    variables = {
      DATABASE_URL = aws_db_instance.postgres.endpoint
      JWT_SECRET   = var.jwt_secret
      JWT_ISSUER   = "banking-api"
      JWT_TTL      = "1h"
    }
  }
}

# Step Functions State Machine
resource "aws_iam_role" "step_functions_role" {
  name = "dating-app-step-functions-role"

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
  name = "dating-app-step-functions-policy"
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
          aws_lambda_function.register_user.arn,
          aws_lambda_function.create_profile.arn,
          aws_lambda_function.match_user.arn,
          aws_lambda_function.send_notification.arn
        ]
      }
    ]
  })
}

resource "aws_sfn_state_machine" "dating_app" {
  name     = "dating-app-workflow"
  role_arn = aws_iam_role.step_functions_role.arn
  definition = templatefile("${path.module}/step-functions/dating-app-workflow.json", {
    register_user_arn = aws_lambda_function.register_user.arn
    create_profile_arn = aws_lambda_function.create_profile.arn
    match_user_arn = aws_lambda_function.match_user.arn
    send_notification_arn = aws_lambda_function.send_notification.arn
  })
}

resource "aws_internet_gateway" "gw" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "dating-app-gw"
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
resource "aws_api_gateway_rest_api" "dating_app" {
  name = "dating-app-api"
}

resource "aws_api_gateway_resource" "register" {
  rest_api_id = aws_api_gateway_rest_api.dating_app.id
  parent_id   = aws_api_gateway_rest_api.dating_app.root_resource_id
  path_part   = "register"
}

resource "aws_api_gateway_method" "register_post" {
  rest_api_id   = aws_api_gateway_rest_api.dating_app.id
  resource_id   = aws_api_gateway_resource.register.id
  http_method   = "POST"
  authorization = "NONE"
}

resource "aws_api_gateway_integration" "register_integration" {
  rest_api_id             = aws_api_gateway_rest_api.dating_app.id
  resource_id             = aws_api_gateway_resource.register.id
  http_method             = aws_api_gateway_method.register_post.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.register_user.invoke_arn
}

resource "aws_lambda_permission" "apigw_lambda" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.register_user.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_rest_api.dating_app.execution_arn}/*/*"
}

resource "aws_api_gateway_deployment" "dating_app" {
  rest_api_id = aws_api_gateway_rest_api.dating_app.id
  stage_name  = "prod"
  depends_on  = [aws_api_gateway_integration.register_integration]
} 