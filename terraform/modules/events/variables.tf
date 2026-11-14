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

variable "export_max_attempts" {
  description = "Cuántas veces se intenta armar una exportación antes de marcarla fallida; la API manda las fallidas a la DLQ. La cola pasa a la DLQ por su cuenta lo que se recibe cinco veces más que esto sin resolverse (por ejemplo, una tarea que muere una y otra vez con el mismo mensaje)."
  type        = number
  default     = 3

  validation {
    condition     = var.export_max_attempts >= 1 && var.export_max_attempts <= 10 && floor(var.export_max_attempts) == var.export_max_attempts
    error_message = "Una exportación se intenta al menos una vez, y la espera entre intentos se duplica: export_max_attempts tiene que ser un entero entre 1 y 10."
  }
}

variable "export_visibility_timeout" {
  description = "Segundos que un mensaje de la cola de exportaciones queda oculto mientras una tarea lo atiende (EXPORTACIONES_VISIBILIDAD). La tarea la extiende mientras trabaja, así que acota cuánto tarda en reaparecer si la tarea muere."
  type        = number
  default     = 120

  validation {
    condition     = var.export_visibility_timeout >= 30 && var.export_visibility_timeout <= 43200 && floor(var.export_visibility_timeout) == var.export_visibility_timeout
    error_message = "La visibilidad es un entero de segundos entre 30 (la tarea tiene que alcanzar a extenderla) y 43200, 12 horas (el máximo de SQS)."
  }
}

variable "export_max_age_seconds" {
  description = "Antigüedad, en segundos, del mensaje más viejo de la cola de exportaciones a partir de la cual se avisa a operación: con el exportador en marcha, nada espera tanto."
  type        = number
  default     = 3600

  validation {
    condition     = var.export_max_age_seconds >= 60
    error_message = "La antigüedad máxima tiene que ser de al menos 60 segundos: el exportador arranca desde cero en varios minutos."
  }
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
