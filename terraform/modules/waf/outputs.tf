output "web_acl_name" {
  description = "Nombre de la web ACL."
  value       = aws_wafv2_web_acl.this.name
}

output "web_acl_arn" {
  description = "ARN de la web ACL."
  value       = aws_wafv2_web_acl.this.arn
}

output "log_group_name" {
  description = "Log group de los pedidos evaluados."
  value       = aws_cloudwatch_log_group.this.name
}
