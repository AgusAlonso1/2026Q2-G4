output "bucket_name" {
  description = "Bucket de logs de auditoría: CloudTrail, flow logs y Config, cada uno bajo su prefijo."
  value       = local.bucket_name
}

output "trail_arn" {
  description = "ARN del trail de CloudTrail."
  value       = aws_cloudtrail.this.arn
}

output "flow_log_id" {
  description = "ID del flow log de la VPC."
  value       = aws_flow_log.vpc.id
}

output "config_rule_names" {
  description = "Reglas de AWS Config."
  value       = sort([for r in aws_config_config_rule.this : r.name])
}

output "budget_name" {
  description = "Presupuesto mensual, o null si no hay direcciones para avisar."
  value       = try(aws_budgets_budget.this[0].name, null)
}
