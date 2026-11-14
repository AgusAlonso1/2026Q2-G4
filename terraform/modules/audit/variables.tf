variable "name_prefix" {
  description = "Prefijo del bucket de logs, el trail, el flow log, las reglas de Config y el presupuesto."
  type        = string
}

variable "kms_key_arn" {
  description = "Clave del proyecto con la que se cifran los logs del trail. Su política tiene que dejar cifrar a CloudTrail."
  type        = string
}

variable "vpc_id" {
  description = "VPC cuyo tráfico registra el flow log."
  type        = string
}

variable "log_retention_days" {
  description = "Días que se guardan los logs de auditoría (CloudTrail, flow logs y Config) en el bucket de logs."
  type        = number
  default     = 90

  validation {
    condition     = var.log_retention_days >= 1
    error_message = "La retención tiene que ser de al menos un día."
  }
}

variable "flow_log_traffic_type" {
  description = "Qué registra el flow log: ACCEPT, REJECT o ALL."
  type        = string
  default     = "ALL"

  validation {
    condition     = contains(["ACCEPT", "REJECT", "ALL"], var.flow_log_traffic_type)
    error_message = "Tiene que ser ACCEPT, REJECT o ALL."
  }
}

variable "config_resource_types" {
  description = "Tipos de recurso que registra AWS Config. Sólo los que evalúan las reglas: Config cobra por cada cambio que registra."
  type        = list(string)
  default = [
    "AWS::S3::Bucket",
    "AWS::RDS::DBInstance",
    "AWS::EC2::SecurityGroup",
    "AWS::EC2::VPC",
    "AWS::EC2::Volume",
  ]
}

variable "budget_limit_usd" {
  description = "Presupuesto mensual en dólares. El del laboratorio es de 50 para toda la materia."
  type        = number
  default     = 50
}

variable "budget_emails" {
  description = "Direcciones que reciben los avisos del presupuesto. Vacío: no se crea el presupuesto."
  type        = list(string)
  default     = []
}
