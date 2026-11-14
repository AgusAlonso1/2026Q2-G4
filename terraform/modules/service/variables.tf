variable "name" {
  description = "Nombre del servicio, de su task definition y de su log group."
  type        = string
}

variable "cluster_name" {
  description = "Cluster de ECS, con el capacity provider del servicio ya asociado."
  type        = string
}

variable "capacity_provider_name" {
  description = "Capacity provider donde se ubican las tareas."
  type        = string
}

variable "image" {
  description = "Imagen del contenedor, con su tag."
  type        = string
}

variable "command" {
  description = "Binario que corre el contenedor dentro de la imagen."
  type        = list(string)
}

variable "cpu" {
  description = "CPU reservada por tarea, en unidades de ECS (1024 = 1 vCPU). Es la base de la métrica de CPU del autoscaling."
  type        = number
}

variable "memory" {
  description = "Memoria máxima por tarea, en MiB."
  type        = number
}

variable "environment" {
  description = "Variables de entorno del contenedor. Sin secretos: quedan a la vista en la task definition."
  type        = map(string)
  default     = {}
}

variable "secrets" {
  description = "Variables secretas: nombre → ARN de Secrets Manager (arn:clave-json::version-id). ECS lee el valor al arrancar la tarea, con el rol de ejecución."
  type        = map(string)
  default     = {}
}

variable "port" {
  description = "Puerto del contenedor. null si el servicio no recibe tráfico."
  type        = number
  default     = null
}

variable "target_group_arn" {
  description = "Target group donde ECS registra las tareas. null si el servicio no va detrás del balanceador."
  type        = string
  default     = null
}

variable "subnet_ids" {
  description = "Subredes de las tareas (red awsvpc: cada tarea tiene su ENI). Las mismas que las de sus instancias."
  type        = list(string)
}

variable "security_group_id" {
  description = "SG de la ENI de cada tarea."
  type        = string
}

variable "role_arn" {
  description = "Rol de ejecución y de la tarea. En AWS Academy no se pueden crear roles: es el del laboratorio."
  type        = string
}

variable "min_count" {
  description = "Mínimo de tareas. También es la cantidad con la que arranca el servicio."
  type        = number
}

variable "max_count" {
  description = "Máximo de tareas."
  type        = number
}

variable "cpu_target" {
  description = "CPU promedio, en %, que mantiene el autoscaling. null para no escalar por CPU."
  type        = number
  default     = null
}

variable "queue_scaling" {
  description = "Escalado por la profundidad de una cola SQS: suma una tarea si hay mensajes visibles (dos desde backlog_step) y baja a min_count cuando la cola lleva idle_minutes sin mensajes visibles, en curso ni demorados. null para no escalar por cola."
  type = object({
    queue_name   = string
    backlog_step = optional(number, 10)
    idle_minutes = optional(number, 10)
  })
  default = null

  validation {
    condition     = var.queue_scaling == null || (try(var.queue_scaling.backlog_step, 0) >= 1 && try(var.queue_scaling.idle_minutes, 0) >= 1)
    error_message = "backlog_step e idle_minutes tienen que ser al menos 1."
  }
}

variable "deployment_minimum_healthy_percent" {
  description = "Tareas que tienen que seguir corriendo durante un deploy, en % de las deseadas. 0 deja que ECS pare las viejas antes de arrancar las nuevas."
  type        = number
  default     = 50
}

variable "deployment_maximum_percent" {
  description = "Tope de tareas durante un deploy, en % de las deseadas. 100 reemplaza sin tareas de más."
  type        = number
  default     = 200
}

variable "schedule" {
  description = "Acciones programadas: fijan la cantidad de tareas a una hora. Cron de Application Auto Scaling."
  type = map(object({
    schedule = string
    tasks    = number
  }))
  default = {}
}

variable "schedule_time_zone" {
  description = "Zona horaria de las acciones programadas."
  type        = string
  default     = "America/Argentina/Buenos_Aires"
}

variable "log_kms_key_arn" {
  description = "Clave KMS con la que se cifran los logs. null: la de CloudWatch."
  type        = string
  default     = null
}

variable "log_retention_days" {
  description = "Días que se conservan los logs del servicio."
  type        = number
  default     = 14

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653], var.log_retention_days)
    error_message = "CloudWatch Logs sólo acepta 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288 o 3653 días."
  }
}
