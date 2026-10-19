output "dns_name" {
  description = "DNS público del NLB: la URL de la API."
  value       = aws_lb.this.dns_name
}

output "target_group_arn" {
  description = "Target group donde el ASG de la API registra sus instancias."
  value       = aws_lb_target_group.api.arn
}
