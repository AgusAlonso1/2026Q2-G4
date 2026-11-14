variable "name_prefix" {
  description = "Prefijo de los nombres: topics, colas, Lambda y secreto."
  type        = string
}

variable "kms_key_arn" {
  description = "Clave KMS de los topics, las colas, los logs y el secreto. Su política tiene que habilitar a sns.amazonaws.com y cloudwatch.amazonaws.com."
  type        = string
}

variable "lambda_role_arn" {
  description = "Rol de ejecución de la Lambda. En AWS Academy, el LabRole preexistente."
  type        = string
}

variable "lambda_runtime" {
  description = "Runtime de la Lambda. Node: el SDK de AWS viene en el runtime, sin dependencias ni paso de build."
  type        = string
  default     = "nodejs22.x"
}

variable "lambda_timeout" {
  description = "Timeout de la Lambda, en segundos. La visibilidad de la cola del webhook es 6 veces este valor."
  type        = number
  default     = 10
}

variable "webhook_source_dir" {
  description = "Carpeta con el handler.mjs de la Lambda del webhook."
  type        = string
}

variable "webhook_url" {
  description = "Adonde se entregan los avisos. Vacía: la Lambda descarta los mensajes."
  type        = string
  default     = ""
}

variable "webhook_secret" {
  description = "Secreto HMAC de los avisos. Tiene que coincidir con el del que recibe."
  type        = string
  sensitive   = true
}

variable "webhook_max_attempts" {
  description = "Cuántas veces se intenta entregar un aviso antes de mandarlo a la DLQ."
  type        = number
  default     = 5
}

variable "notification_emails" {
  description = "Emails que se suscriben al topic de alertas de operación."
  type        = list(string)
  default     = []
}

variable "log_retention_days" {
  description = "Días que se conservan los logs de la Lambda."
  type        = number
  default     = 7
}
