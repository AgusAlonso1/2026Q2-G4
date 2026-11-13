output "topic_arn" {
  description = "Topic donde la API publica cada orden terminada (SNS_TOPIC_ARN)."
  value       = aws_sns_topic.events.arn
}

output "alerts_topic_arn" {
  description = "Topic de alertas de operación: la alarma de la DLQ del webhook."
  value       = aws_sns_topic.alerts.arn
}

output "webhook_queue_url" {
  description = "Cola de entrega del webhook."
  value       = aws_sqs_queue.webhook.url
}

output "webhook_dlq_url" {
  description = "DLQ del webhook: los avisos que agotaron los reintentos."
  value       = aws_sqs_queue.webhook_dlq.url
}
