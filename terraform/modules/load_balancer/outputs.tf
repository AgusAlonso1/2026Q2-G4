output "dns_name" {
  description = "DNS público del NLB: la URL de la API."
  value       = aws_lb.this.dns_name
}

output "target_group_arn" {
  description = "Target group donde ECS registra las tareas de la API, por IP. Sale del listener: así el servicio se crea recién cuando el target group ya está atado al NLB."
  value       = aws_lb_listener.http.default_action[0].target_group_arn
}

output "arn_suffix" {
  description = "Sufijo del ARN del NLB (net/<nombre>/<id>): la dimensión LoadBalancer de sus métricas en CloudWatch."
  value       = aws_lb.this.arn_suffix
}

output "target_group_arn_suffix" {
  description = "Sufijo del ARN del target group de la API (targetgroup/<nombre>/<id>): la dimensión TargetGroup de sus métricas."
  value       = aws_lb_target_group.api.arn_suffix
}
