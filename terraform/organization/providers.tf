provider "aws" {
  region = var.aws_region

  default_tags {
    tags = local.common_tags
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}

# AWS Academy no permite crear IAM: las instancias usan el perfil que trae el laboratorio.
data "aws_iam_instance_profile" "lab" {
  name = var.instance_profile_name
}

# Mismo motivo: la Lambda del callback de Cognito corre con el rol del laboratorio.
data "aws_iam_role" "lab" {
  name = var.lambda_role_name
}

data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-x86_64"]
  }
}
