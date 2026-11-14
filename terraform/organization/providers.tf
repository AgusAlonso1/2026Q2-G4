provider "aws" {
  region = var.aws_region

  default_tags {
    tags = local.common_tags
  }
}

# La otra región que habilita AWS Academy: ahí van las copias de los backups (Disaster Recovery).
provider "aws" {
  alias  = "dr"
  region = var.dr_region

  default_tags {
    tags = local.common_tags
  }
}

# Sin default_tags: AWS Academy deja crear roles vinculados a servicios pero no etiquetarlos
# (iam:TagRole), y el provider etiqueta todo lo que crea. Sólo lo usa el rol de AWS Config.
provider "aws" {
  alias  = "sin_tags"
  region = var.aws_region
}

data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_caller_identity" "current" {}

# AWS Academy no permite crear IAM: las instancias usan el perfil que trae el laboratorio.
data "aws_iam_instance_profile" "lab" {
  name = var.instance_profile_name
}

# Mismo motivo: la Lambda del callback de Cognito y las tareas de ECS corren con el rol del laboratorio.
data "aws_iam_role" "lab" {
  name = var.lab_role_name
}
