variable "name_prefix" {
  description = "Prefijo de los nombres."
  type        = string
}

variable "resource_arn" {
  description = "ARN del user pool de Cognito."
  type        = string
}

variable "kms_key_arn" {
  description = "Clave KMS de los logs."
  type        = string
}

variable "rate_limit" {
  description = "Pedidos por IP cada 5 minutos."
  type        = number
  default     = 100

  validation {
    condition     = var.rate_limit >= 10
    error_message = "WAF no acepta un límite menor a 10."
  }
}

variable "log_retention_days" {
  description = "Días que se conservan los logs."
  type        = number
  default     = 7
}
