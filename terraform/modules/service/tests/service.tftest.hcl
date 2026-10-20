# El módulo solo, con un provider mock: cómo arma la task definition y qué escalado crea
# según reciba tráfico (la API) o corra por horario (el archivador).

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = { region = "us-east-1" }
  }
}

variables {
  name                   = "ledger-api"
  cluster_name           = "ledger-cluster"
  capacity_provider_name = "ledger-api"
  image                  = "123456789012.dkr.ecr.us-east-1.amazonaws.com/ledger:abc123"
  command                = ["/api"]
  cpu                    = 1024
  memory                 = 512
  environment            = { S3_BUCKET = "ledger-bucket", APP_PORT = "8080" }
  secrets                = { DB_URL = "arn:aws:secretsmanager:us-east-1:123456789012:secret:ledger/api-AbCdEf:DB_URL::v1" }
  log_kms_key_arn        = "arn:aws:kms:us-east-1:123456789012:key/mock"
  port                   = 8080
  target_group_arn       = "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/ledger-tg-api/mock"
  subnet_ids             = ["subnet-a", "subnet-b"]
  security_group_id      = "sg-app"
  role_arn               = "arn:aws:iam::123456789012:role/LabRole"
  min_count              = 2
  max_count              = 6
  cpu_target             = 60
}

run "api_detras_del_balanceador" {
  command = apply

  assert {
    condition     = jsonencode(jsondecode(aws_ecs_task_definition.this.container_definitions)[0].secrets) == jsonencode([{ name = "DB_URL", valueFrom = var.secrets["DB_URL"] }])
    error_message = "Los secretos van como valueFrom, no como valor."
  }

  assert {
    condition     = join(",", [for e in jsondecode(aws_ecs_task_definition.this.container_definitions)[0].environment : e.name]) == "APP_PORT,S3_BUCKET"
    error_message = "environment va ordenado por nombre (sin diffs espurios en el plan) y sin secretos."
  }

  assert {
    condition     = aws_cloudwatch_log_group.this.kms_key_id == var.log_kms_key_arn
    error_message = "Los logs se cifran con la clave recibida."
  }

  assert {
    condition     = length(aws_ecs_service.this.load_balancer) == 1 && length(aws_appautoscaling_policy.cpu) == 1
    error_message = "Con target group y cpu_target: va detrás del balanceador y escala por CPU."
  }
}

run "archivador_por_horario" {
  command = apply
  # Estado propio: sobre el del run anterior, desired_count quedaría en 2 por el
  # ignore_changes del servicio (que es justamente lo que tiene que hacer).
  state_key = "archivador"

  variables {
    name             = "ledger-archive"
    command          = ["/archivador"]
    port             = null
    target_group_arn = null
    cpu_target       = null
    min_count        = 0
    max_count        = 1
    schedule = {
      abrir  = { schedule = "cron(25 19 * * ? *)", tasks = 1 }
      cerrar = { schedule = "cron(30 21 * * ? *)", tasks = 0 }
    }
  }

  assert {
    condition     = length(aws_ecs_service.this.load_balancer) == 0 && length(aws_appautoscaling_policy.cpu) == 0
    error_message = "Sin puerto ni cpu_target: ni balanceador ni escalado por CPU."
  }

  assert {
    condition     = aws_ecs_service.this.desired_count == 0 && length(jsondecode(aws_ecs_task_definition.this.container_definitions)[0].portMappings) == 0
    error_message = "El archivador arranca en cero tareas y no expone puertos."
  }

  assert {
    condition     = aws_appautoscaling_scheduled_action.this["abrir"].timezone == "America/Argentina/Buenos_Aires"
    error_message = "Las acciones programadas van en hora de Buenos Aires."
  }
}
