resource "aws_sns_topic" "events" {
  name              = "${var.name_prefix}-events"
  kms_master_key_id = var.kms_key_arn
}

resource "aws_sqs_queue" "webhook_dlq" {
  name                      = "${var.name_prefix}-webhook-dlq"
  kms_master_key_id         = var.kms_key_arn
  message_retention_seconds = 1209600
}

resource "aws_sqs_queue" "webhook" {
  name                       = "${var.name_prefix}-webhook"
  kms_master_key_id          = var.kms_key_arn
  visibility_timeout_seconds = 6 * var.lambda_timeout

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.webhook_dlq.arn
    maxReceiveCount     = var.webhook_max_attempts
  })
}

resource "aws_sqs_queue_policy" "webhook" {
  queue_url = aws_sqs_queue.webhook.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "FromEventsTopic"
      Effect    = "Allow"
      Principal = { Service = "sns.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.webhook.arn
      Condition = { ArnEquals = { "aws:SourceArn" = aws_sns_topic.events.arn } }
    }]
  })
}

resource "aws_sns_topic_subscription" "webhook" {
  topic_arn            = aws_sns_topic.events.arn
  protocol             = "sqs"
  endpoint             = aws_sqs_queue.webhook.arn
  raw_message_delivery = true

  depends_on = [aws_sqs_queue_policy.webhook]
}

resource "aws_lambda_event_source_mapping" "webhook" {
  event_source_arn        = aws_sqs_queue.webhook.arn
  function_name           = aws_lambda_function.webhook.arn
  batch_size              = 10
  function_response_types = ["ReportBatchItemFailures"]
}

resource "aws_secretsmanager_secret" "webhook" {
  name                    = "${var.name_prefix}/webhook"
  description             = "Secreto HMAC con el que la Lambda del webhook firma los avisos"
  kms_key_id              = var.kms_key_arn
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "webhook" {
  secret_id     = aws_secretsmanager_secret.webhook.id
  secret_string = var.webhook_secret
}

data "archive_file" "webhook" {
  type        = "zip"
  source_dir  = var.webhook_source_dir
  output_path = "${path.root}/.build/${var.name_prefix}-webhook.zip"
}

resource "aws_cloudwatch_log_group" "webhook" {
  name              = "/aws/lambda/${var.name_prefix}-webhook"
  retention_in_days = var.log_retention_days
  kms_key_id        = var.kms_key_arn
}

resource "aws_lambda_function" "webhook" {
  function_name    = "${var.name_prefix}-webhook"
  description      = "Lee la cola del webhook, firma cada aviso y lo entrega"
  role             = var.lambda_role_arn
  runtime          = var.lambda_runtime
  handler          = "handler.handler"
  filename         = data.archive_file.webhook.output_path
  source_code_hash = data.archive_file.webhook.output_base64sha256
  timeout          = var.lambda_timeout

  environment {
    variables = {
      WEBHOOK_URL        = var.webhook_url
      WEBHOOK_SECRET_ARN = aws_secretsmanager_secret.webhook.arn
    }
  }

  depends_on = [aws_cloudwatch_log_group.webhook]
}

# Antes eran dos Lambdas con for_each (webhook y mailer). Sin esto, Terraform borraría y
# volvería a crear la del webhook y su log group en los despliegues que ya existen.
moved {
  from = aws_lambda_function.this["webhook"]
  to   = aws_lambda_function.webhook
}

moved {
  from = aws_cloudwatch_log_group.this["webhook"]
  to   = aws_cloudwatch_log_group.webhook
}

resource "aws_sns_topic" "alerts" {
  name              = "${var.name_prefix}-alerts"
  kms_master_key_id = var.kms_key_arn
}

resource "aws_sns_topic_subscription" "alerts" {
  for_each = toset(var.notification_emails)

  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = each.value
}

resource "aws_cloudwatch_metric_alarm" "webhook_dlq" {
  alarm_name          = "${var.name_prefix}-webhook-dlq"
  alarm_description   = "Hay avisos de webhook que agotaron los reintentos y quedaron en la DLQ"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = aws_sqs_queue.webhook_dlq.name }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alerts.arn]
}
