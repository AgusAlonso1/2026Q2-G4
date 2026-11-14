variable "name_prefix" {
  description = "Prefijo de los nombres: workgroup de Athena y base del catálogo de Glue (con los guiones pasados a guion bajo)."
  type        = string
}

variable "bucket_name" {
  description = "Bucket del proyecto: de ahí lee el archivo y ahí escribe Athena los resultados."
  type        = string
}

variable "kms_key_arn" {
  description = "Clave KMS con la que se cifran los resultados de las consultas (SSE-KMS)."
  type        = string
}

variable "history_prefix" {
  description = "Prefijo del archivo del ledger en el bucket. Debajo, un prefijo periodo=AAAA-MM por mes archivado."
  type        = string
  default     = "transaction-history/"

  validation {
    condition     = endswith(var.history_prefix, "/")
    error_message = "history_prefix tiene que terminar en /."
  }
}

variable "results_prefix" {
  description = "Prefijo del bucket donde Athena escribe los resultados de las consultas."
  type        = string
  default     = "athena-results/"

  validation {
    condition     = endswith(var.results_prefix, "/")
    error_message = "results_prefix tiene que terminar en /."
  }
}

variable "history_start" {
  description = "Primer mes (AAAA-MM) que proyecta la tabla. Los meses anteriores no se ven en Athena."
  type        = string
  default     = "2024-01"

  validation {
    condition     = can(regex("^[0-9]{4}-(0[1-9]|1[0-2])$", var.history_start))
    error_message = "history_start tiene que tener formato AAAA-MM."
  }
}

variable "bytes_scanned_cutoff_per_query" {
  description = "Tope de bytes que puede escanear una consulta del workgroup: la cancela si lo pasa. Athena cobra por byte escaneado."
  type        = number
  default     = 1073741824

  validation {
    condition     = var.bytes_scanned_cutoff_per_query >= 10485760
    error_message = "Athena no acepta un tope menor a 10 MiB (10485760 bytes)."
  }
}
