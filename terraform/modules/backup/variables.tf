variable "name_prefix" {
  description = "Prefijo de los vaults, el plan y la selección."
  type        = string
}

variable "db_instance_arn" {
  description = "Instancia RDS que se respalda."
  type        = string
}

variable "role_arn" {
  description = "Rol con el que AWS Backup hace las copias. En AWS Academy, LabRole: confía en backup.amazonaws.com."
  type        = string
}

variable "schedule" {
  description = "Cuándo corre el backup diario. Cron en UTC: las 7 son las 4 de Buenos Aires, fuera del horario de pagos y antes de la ventana del archivador."
  type        = string
  default     = "cron(0 7 * * ? *)"
}

variable "retention_days" {
  description = "Días que se conserva cada copia, en la región principal y en la de DR."
  type        = number
  default     = 7

  validation {
    condition     = var.retention_days >= 1
    error_message = "La retención tiene que ser de al menos un día."
  }
}
