variable "name_prefix" {
  description = "Prefijo del nombre del bucket."
  type        = string
}

variable "artifact_path" {
  description = "Ruta local al tar.gz con los binarios de la API y del proceso archivador."
  type        = string
}

variable "noncurrent_version_days" {
  description = "Dias que se conserva una versión remplazada o borrada antes de eliminarla."
  type        = number
  default     = 30
}

variable "transitions" {
  description = "Transiciones de clases de almacenamiento."
  type = map(object({
    prefix = string
    transitions = list(object({
      days          = number
      storage_class = string
    }))
  }))
  default = {
    "transaction-history" = {
      prefix = "transaction-history/"
      transitions = [
        { days = 30, storage_class = "STANDARD_IA" },
        { days = 365, storage_class = "GLACIER" },
      ]
    }
  }
}

