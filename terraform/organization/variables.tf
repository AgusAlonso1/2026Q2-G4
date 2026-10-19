variable "project_name" {
  description = "Prefijo de todos los recursos (ledger-vpc, ledger-nlb, ...)."
  type        = string
  default     = "ledger"
}

variable "aws_region" {
  description = "Región de despliegue. AWS Academy sólo habilita us-east-1."
  type        = string
  default     = "us-east-1"
}

variable "vpc_cidr" {
  description = "CIDR de la VPC. Las subredes se derivan de él en locals.tf."
  type        = string
  default     = "10.0.0.0/16"
}

variable "app_port" {
  description = "Puerto donde escucha la API. Lo usan el SG, el target group y el user_data."
  type        = number
  default     = 8080
}

variable "db_port" {
  description = "Puerto de PostgreSQL. Lo usan los SG y la instancia RDS."
  type        = number
  default     = 5432
}

variable "instance_profile_name" {
  description = "Instance profile preexistente del laboratorio."
  type        = string
  default     = "LabInstanceProfile"
}

variable "lambda_role_name" {
  description = "Rol preexistente del laboratorio con el que corre la Lambda del callback de Cognito."
  type        = string
  default     = "LabRole"
}

variable "instance_type" {
  description = "Tipo de instancia de los dos ASG."
  type        = string
  default     = "t3.micro"
}

variable "db_engine_version" {
  description = "Versión de PostgreSQL. Sólo el major: RDS elige el último minor disponible."
  type        = string
  default     = "18"
}

variable "db_instance_class" {
  description = "Clase de la instancia RDS. La más chica: la demo es funcional, no de carga."
  type        = string
  default     = "db.t3.micro"
}

variable "db_multi_az" {
  description = "Standby en la segunda AZ: la base sigue disponible si se cae una zona. En false el apply tarda la mitad: sólo para probar."
  type        = bool
  default     = true
}

# TODO: variables del resto de la arquitectura a medida que se llenan los módulos, por ejemplo
#   - mock_api_key / webhook_secret (los secretos que SÍ se comparten con el otro equipo)
#   - proveedor_url / webhook_url
#   - archive_schedule (cron de apertura y cierre de la ventana de archivado)
