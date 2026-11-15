output "dashboard_url" {
  description = "Dashboard de CloudWatch con el cómputo, la entrada, la base y los eventos."
  value       = "https://${local.region}.console.aws.amazon.com/cloudwatch/home?region=${local.region}#dashboards/dashboard/${aws_cloudwatch_dashboard.this.dashboard_name}"
}

output "alarm_names" {
  description = "Alarmas que crea el módulo. Publican en el topic de alertas al dispararse y al volver a OK."
  value       = sort([for a in aws_cloudwatch_metric_alarm.this : a.alarm_name])
}

output "metric_namespace" {
  description = "Namespace de las métricas que salen del log de la API (Pedidos, Latencia, Respuestas5xx)."
  value       = local.api_namespace
}

output "query_names" {
  description = "Consultas guardadas de Logs Insights."
  value       = sort([for q in aws_cloudwatch_query_definition.this : q.name])
}
