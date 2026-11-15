variable "name_prefix" {
  description = "Prefijo del bucket de logs, el trail y el presupuesto."
  type        = string
}

variable "kms_key_arn" {
  description = "Clave del proyecto con la que se cifran los logs del trail. Su política tiene que dejar cifrar a CloudTrail."
  type        = string
}

variable "log_retention_days" {
  description = "Días que se guardan los logs del trail en el bucket de logs."
  type        = number
  default     = 90

  validation {
    condition     = var.log_retention_days >= 1
    error_message = "La retención tiene que ser de al menos un día."
  }
}

variable "budget_limit_usd" {
  description = "Presupuesto mensual en dólares. El crédito del laboratorio es de 50 para toda la materia."
  type        = number
  default     = 50

  validation {
    condition     = var.budget_limit_usd > 0
    error_message = "El presupuesto tiene que ser mayor que cero."
  }
}

variable "budget_emails" {
  description = "Direcciones que reciben los avisos del presupuesto. Vacío: no se crea el presupuesto."
  type        = list(string)
  default     = []
}
