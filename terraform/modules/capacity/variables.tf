variable "name" {
  description = "Nombre del launch template, el ASG y el capacity provider."
  type        = string
}

variable "cluster_name" {
  description = "Cluster de ECS al que se unen las instancias."
  type        = string
}

variable "subnet_ids" {
  description = "Subredes de las instancias. El agente llega a ECS, ECR y CloudWatch Logs por los interface endpoints."
  type        = list(string)
}

variable "ami_id" {
  description = "AMI optimizada para ECS: ya trae Docker y el agente."
  type        = string
}

variable "instance_type" {
  description = "Tipo de instancia. Con red awsvpc, una t3.micro aloja una sola tarea."
  type        = string
}

variable "instance_profile_arn" {
  description = "Instance profile de las instancias. Lo necesita el agente para registrarse en el cluster."
  type        = string
}

variable "security_group_id" {
  description = "SG de las instancias. Las tareas usan el suyo propio."
  type        = string
}

variable "max_size" {
  description = "Máximo de instancias: una por tarea, más una para la tarea nueva durante un deploy."
  type        = number
}
