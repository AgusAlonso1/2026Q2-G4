output "vault_name" {
  description = "Vault principal, en la región del despliegue."
  value       = aws_backup_vault.primary.name
}

output "dr_vault_name" {
  description = "Vault de DR, en la otra región."
  value       = aws_backup_vault.dr.name
}

output "dr_region" {
  description = "Región de las copias de DR."
  value       = data.aws_region.dr.region
}

output "plan_id" {
  description = "Plan de backup: para correr uno a demanda con start-backup-job."
  value       = aws_backup_plan.this.id
}
