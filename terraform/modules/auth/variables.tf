variable "name_prefix" {
  description = "Prefijo de los nombres: pool, dominio de la Hosted UI, Lambda y API."
  type        = string
}

variable "frontend_url" {
  description = "URL HTTP del frontend: adonde la Lambda redirige con los tokens en el fragment."
  type        = string
}

variable "lambda_role_arn" {
  description = "Rol de ejecución de la Lambda. En AWS Academy, el LabRole preexistente."
  type        = string
}

variable "callback_source_dir" {
  description = "Carpeta con el handler.mjs de la Lambda del callback."
  type        = string
}

variable "lambda_runtime" {
  description = "Runtime de la Lambda del callback. Node: el handler no tiene dependencias ni paso de build."
  type        = string
  default     = "nodejs22.x"
}

variable "self_sign_up" {
  description = "Si la Hosted UI deja registrarse. En false, los usuarios los crea un admin."
  type        = bool
  default     = true
}

variable "token_validity_hours" {
  description = "Vida del access token y del ID token, en horas (1 a 24). No se entrega refresh token al frontend."
  type        = number
  default     = 8

  validation {
    condition     = var.token_validity_hours >= 1 && var.token_validity_hours <= 24
    error_message = "Cognito acepta entre 1 y 24 horas."
  }
}

variable "log_retention_days" {
  description = "Días que se conservan los logs de la Lambda."
  type        = number
  default     = 7
}
