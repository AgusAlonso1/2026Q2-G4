variable "name_prefix" {
  description = "Prefijo de las alarmas, el dashboard, las métricas y las consultas guardadas."
  type        = string
}

variable "alarm_topic_arn" {
  description = "Topic SNS de alertas de operación: las alarmas publican ahí cuando se disparan y cuando vuelven a OK."
  type        = string
}

variable "cluster_name" {
  description = "Cluster de ECS. Container Insights publica sus métricas con esta dimensión."
  type        = string
}

variable "api_service_name" {
  description = "Servicio de ECS de la API."
  type        = string
}

variable "archive_service_name" {
  description = "Servicio de ECS del archivador."
  type        = string
}

variable "api_min_tasks" {
  description = "Mínimo de tareas de la API. Con menos targets sanos en el NLB, la alarma se dispara."
  type        = number
}

variable "api_cpu_target" {
  description = "CPU objetivo del autoscaling de la API, en %. Se dibuja como referencia en el dashboard."
  type        = number
}

variable "api_log_group_name" {
  description = "Log group de la API. Cada request deja una línea JSON con msg = \"request\", ruta, estado y ms."
  type        = string
}

variable "archive_log_group_name" {
  description = "Log group del archivador, para su consulta guardada."
  type        = string
}

variable "webhook_log_group_name" {
  description = "Log group de la Lambda del webhook, para su consulta guardada."
  type        = string
}

variable "webhook_function_name" {
  description = "Lambda del webhook."
  type        = string
}

variable "webhook_queue_name" {
  description = "Cola SQS del webhook."
  type        = string
}

variable "webhook_dlq_name" {
  description = "DLQ del webhook."
  type        = string
}

variable "nlb_arn_suffix" {
  description = "Sufijo del ARN del NLB (net/<nombre>/<id>): la dimensión LoadBalancer de sus métricas."
  type        = string
}

variable "target_group_arn_suffix" {
  description = "Sufijo del ARN del target group de la API (targetgroup/<nombre>/<id>): la dimensión TargetGroup."
  type        = string
}

variable "db_instance_identifier" {
  description = "Identificador de la instancia RDS."
  type        = string
}

variable "waf_web_acl_name" {
  description = "Web ACL del WAF del login, para el panel de pedidos bloqueados."
  type        = string
}

variable "latency_p99_threshold_ms" {
  description = "Latencia p99 de la API, en ms, por encima de la cual se dispara la alarma (3 minutos seguidos). No cuenta /health."
  type        = number
  default     = 1000
}

variable "errors_5xx_threshold" {
  description = "Respuestas 5xx de la API en 5 minutos a partir de las cuales se dispara la alarma."
  type        = number
  default     = 5
}

variable "db_cpu_threshold" {
  description = "CPU de RDS, en %, sostenida 10 minutos."
  type        = number
  default     = 80
}

variable "db_connections_threshold" {
  description = "Conexiones abiertas a RDS. El diseño reparte db_max_connections (60) y una db.t3.micro acepta unas 80: pasar este valor quiere decir que algo se salió del cálculo."
  type        = number
  default     = 70
}

variable "db_free_memory_threshold_mb" {
  description = "Memoria libre de RDS, en MB, por debajo de la cual se dispara la alarma (10 minutos)."
  type        = number
  default     = 100
}

variable "db_free_storage_threshold_gb" {
  description = "Espacio libre en el disco de RDS, en GB. El disco es fijo, sin autoescalado de almacenamiento."
  type        = number
  default     = 2
}
