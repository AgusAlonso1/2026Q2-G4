output "topic_arn" {
  description = "Topic donde la API publica cada orden terminada (SNS_TOPIC_ARN)."
  value       = aws_sns_topic.events.arn
}

output "titulares_topic_arn" {
  description = "Topic de los avisos por mail a cada titular (TITULARES_TOPIC_ARN): cada suscripción filtra por su titular."
  value       = aws_sns_topic.titulares.arn
}

output "alerts_topic_arn" {
  description = "Topic de alertas de operación: las alarmas de las DLQ del webhook y de las exportaciones, y la de antigüedad de la cola de exportaciones."
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

output "exports_queue_url" {
  description = "Cola de las exportaciones pendientes (EXPORTACIONES_QUEUE_URL): la API encola cada pedido y la consume el exportador."
  value       = aws_sqs_queue.exports.url
}

output "exports_queue_name" {
  description = "Nombre de la cola de exportaciones: la dimensión de las métricas con las que escala el exportador."
  value       = aws_sqs_queue.exports.name
}

output "exports_dlq_url" {
  description = "DLQ de las exportaciones (EXPORTACIONES_DLQ_URL): las que agotaron los reintentos y los mensajes que la cola no pudo entregar."
  value       = aws_sqs_queue.exports_dlq.url
}

output "queue_names" {
  description = "Nombres de las colas y sus DLQ: la dimensión QueueName de sus métricas en CloudWatch."
  value = {
    webhook     = aws_sqs_queue.webhook.name
    webhook_dlq = aws_sqs_queue.webhook_dlq.name
    exports     = aws_sqs_queue.exports.name
    exports_dlq = aws_sqs_queue.exports_dlq.name
  }
}

output "webhook_function_name" {
  description = "Lambda del webhook."
  value       = aws_lambda_function.this["webhook"].function_name
}

output "webhook_log_group_name" {
  description = "Log group de la Lambda del webhook."
  value       = aws_cloudwatch_log_group.this["webhook"].name
}

output "alarm_names" {
  description = "Alarmas de las colas: la DLQ del webhook y las de las exportaciones."
  value = sort([
    aws_cloudwatch_metric_alarm.webhook_dlq.alarm_name,
    aws_cloudwatch_metric_alarm.exports_age.alarm_name,
    aws_cloudwatch_metric_alarm.exports_dlq.alarm_name,
    aws_cloudwatch_metric_alarm.exports_dlq_sent.alarm_name,
  ])
}
