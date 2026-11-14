output "bucket_name" {
  description = "Bucket de logs del trail."
  value       = local.bucket_name
}

output "trail_arn" {
  description = "ARN del trail de CloudTrail."
  value       = aws_cloudtrail.this.arn
}

output "budget_name" {
  description = "Presupuesto mensual, o null si no hay direcciones para avisar."
  value       = try(aws_budgets_budget.this[0].name, null)
}
