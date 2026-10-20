# Pruebas sin AWS: el provider es un mock que inventa los atributos de cada recurso, así que
# `terraform test` recorre el grafo completo (dependencias, for_each, valores sensibles) y
# "aplica" todo sin credenciales ni costo. Lo que se verifica es la configuración, no AWS.
#
# Los atributos que el provider valida por formato (ARN, id de launch template) necesitan un
# valor con la forma correcta; el resto lo genera el mock.

mock_provider "aws" {
  mock_data "aws_availability_zones" {
    defaults = { names = ["us-east-1a", "us-east-1b", "us-east-1c"] }
  }
  mock_data "aws_caller_identity" {
    defaults = { account_id = "123456789012" }
  }
  mock_data "aws_region" {
    defaults = { region = "us-east-1", name = "us-east-1" }
  }
  mock_data "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::123456789012:role/LabRole" }
  }
  mock_data "aws_iam_instance_profile" {
    defaults = { arn = "arn:aws:iam::123456789012:instance-profile/LabInstanceProfile" }
  }
  mock_data "aws_ssm_parameter" {
    defaults = { insecure_value = "ami-0123456789abcdef0" }
  }

  mock_resource "aws_kms_key" {
    defaults = { arn = "arn:aws:kms:us-east-1:123456789012:key/mock" }
  }
  mock_resource "aws_secretsmanager_secret" {
    defaults = { arn = "arn:aws:secretsmanager:us-east-1:123456789012:secret:ledger/mock-AbCdEf" }
  }
  mock_resource "aws_secretsmanager_secret_version" {
    defaults = { version_id = "mock-version" }
  }
  mock_resource "aws_launch_template" {
    defaults = { id = "lt-0123456789abcdef0" }
  }
  mock_resource "aws_autoscaling_group" {
    defaults = { arn = "arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:mock:autoScalingGroupName/mock" }
  }
  mock_resource "aws_lb" {
    defaults = {
      arn      = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/net/ledger-nlb/mock"
      dns_name = "ledger-nlb-mock.elb.us-east-1.amazonaws.com"
    }
  }
  mock_resource "aws_lb_target_group" {
    defaults = { arn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/ledger-tg-api/mock" }
  }
  mock_resource "aws_apigatewayv2_api" {
    defaults = {
      execution_arn = "arn:aws:execute-api:us-east-1:123456789012:mock"
      api_endpoint  = "https://mock.execute-api.us-east-1.amazonaws.com"
    }
  }
  mock_resource "aws_lambda_function" {
    defaults = {
      arn        = "arn:aws:lambda:us-east-1:123456789012:function:ledger-callback"
      invoke_arn = "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/arn:aws:lambda:us-east-1:123456789012:function:ledger-callback/invocations"
    }
  }
  mock_resource "aws_s3_bucket" {
    defaults = { arn = "arn:aws:s3:::ledger-mock" }
  }
}

run "despliegue_completo" {
  command = apply

  assert {
    condition     = length(local.azs) == 2
    error_message = "Toma exactamente dos AZ aunque la región tenga más."
  }

  assert {
    condition = local.subnet_cidrs == {
      public  = ["10.0.0.0/24", "10.0.1.0/24"]
      app     = ["10.0.10.0/24", "10.0.11.0/24"]
      archive = ["10.0.12.0/24", "10.0.13.0/24"]
      data    = ["10.0.20.0/24", "10.0.21.0/24"]
    }
    error_message = "Los CIDR de las capas no son los del diseño."
  }

  assert {
    condition     = join(",", sort(keys(jsondecode(nonsensitive(aws_secretsmanager_secret_version.this["api"].secret_string))))) == "ADMIN_API_KEY,DB_URL,MOCK_API_KEY,PROVEEDOR_API_KEY,WEBHOOK_SECRET"
    error_message = "El secreto de la API no tiene sus cinco claves."
  }

  assert {
    condition     = join(",", keys(jsondecode(nonsensitive(aws_secretsmanager_secret_version.this["archive"].secret_string)))) == "DB_URL"
    error_message = "El secreto del archivador tiene que tener sólo la DB_URL."
  }

  assert {
    condition     = alltrue([for s in aws_secretsmanager_secret.this : s.kms_key_id == aws_kms_key.this.arn && s.recovery_window_in_days == 0])
    error_message = "Cada secreto va cifrado con la clave del proyecto y sin ventana de recuperación."
  }

  assert {
    condition     = alltrue([for k, v in local.services : length(setintersection(keys(v.environment), keys(local.secret_values[k]))) == 0])
    error_message = "Una variable secreta quedó también en environment, a la vista en la task definition."
  }

  assert {
    condition     = contains(keys(aws_vpc_endpoint.interface), "secretsmanager")
    error_message = "Sin el endpoint de Secrets Manager, las tareas del archivador no arrancan."
  }

  assert {
    condition     = aws_ecr_repository.ledger.encryption_configuration[0].kms_key == aws_kms_key.this.arn && aws_ecr_repository.ledger.image_scanning_configuration[0].scan_on_push
    error_message = "ECR tiene que cifrar con la clave del proyecto y escanear al subir."
  }

  assert {
    condition     = output.api_url == "http://ledger-nlb-mock.elb.us-east-1.amazonaws.com"
    error_message = "La URL de la API sale del DNS del NLB."
  }
}

run "archivador_va_de_0_a_1" {
  command = plan

  variables {
    archive_schedule = {
      abrir = { schedule = "cron(25 19 * * ? *)", tasks = 2 }
    }
  }

  expect_failures = [var.archive_schedule]
}
