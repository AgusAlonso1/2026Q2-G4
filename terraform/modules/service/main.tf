data "aws_region" "current" {}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.name}"
  retention_in_days = var.log_retention_days
  kms_key_id        = var.log_kms_key_arn
}

resource "aws_ecs_task_definition" "this" {
  family                   = var.name
  requires_compatibilities = ["EC2"]
  network_mode             = "awsvpc"
  execution_role_arn       = var.role_arn
  task_role_arn            = var.role_arn

  container_definitions = jsonencode([{
    name        = var.name
    image       = var.image
    command     = var.command
    essential   = true
    cpu         = var.cpu
    memory      = var.memory
    stopTimeout = 30
    portMappings = var.port == null ? [] : [{
      containerPort = var.port
      protocol      = "tcp"
    }]
    environment = [for nombre in sort(keys(var.environment)) : { name = nombre, value = var.environment[nombre] }]
    secrets     = [for nombre in sort(keys(var.secrets)) : { name = nombre, valueFrom = var.secrets[nombre] }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.this.name
        awslogs-region        = data.aws_region.current.region
        awslogs-stream-prefix = var.name
      }
    }
  }])
}

resource "aws_ecs_service" "this" {
  name                              = var.name
  cluster                           = var.cluster_name
  task_definition                   = aws_ecs_task_definition.this.arn
  desired_count                     = var.min_count
  health_check_grace_period_seconds = var.target_group_arn == null ? null : 90

  deployment_minimum_healthy_percent = var.deployment_minimum_healthy_percent
  deployment_maximum_percent         = var.deployment_maximum_percent

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  capacity_provider_strategy {
    capacity_provider = var.capacity_provider_name
    weight            = 1
  }

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = [var.security_group_id]
    assign_public_ip = false
  }

  ordered_placement_strategy {
    type  = "spread"
    field = "attribute:ecs.availability-zone"
  }

  dynamic "load_balancer" {
    for_each = var.target_group_arn == null ? [] : [var.target_group_arn]
    content {
      target_group_arn = load_balancer.value
      container_name   = var.name
      container_port   = var.port
    }
  }

  lifecycle {
    ignore_changes = [desired_count]
  }
}

resource "aws_appautoscaling_target" "this" {
  service_namespace  = "ecs"
  resource_id        = "service/${var.cluster_name}/${aws_ecs_service.this.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  min_capacity       = var.min_count
  max_capacity       = var.max_count
}

resource "aws_appautoscaling_policy" "cpu" {
  count = var.cpu_target == null ? 0 : 1

  name               = "${var.name}-cpu"
  service_namespace  = aws_appautoscaling_target.this.service_namespace
  resource_id        = aws_appautoscaling_target.this.resource_id
  scalable_dimension = aws_appautoscaling_target.this.scalable_dimension
  policy_type        = "TargetTrackingScaling"

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = var.cpu_target
    scale_in_cooldown  = 300
    scale_out_cooldown = 90
  }
}

resource "aws_appautoscaling_scheduled_action" "this" {
  for_each = var.schedule

  name               = "${var.name}-${each.key}"
  service_namespace  = aws_appautoscaling_target.this.service_namespace
  resource_id        = aws_appautoscaling_target.this.resource_id
  scalable_dimension = aws_appautoscaling_target.this.scalable_dimension
  schedule           = each.value.schedule
  timezone           = var.schedule_time_zone

  scalable_target_action {
    min_capacity = each.value.tasks
    max_capacity = each.value.tasks
  }
}

locals {
  queue_metrics = {
    visibles  = "ApproximateNumberOfMessagesVisible"
    en_curso  = "ApproximateNumberOfMessagesNotVisible"
    demorados = "ApproximateNumberOfMessagesDelayed"
  }
}

resource "aws_appautoscaling_policy" "queue_out" {
  count = var.queue_scaling == null ? 0 : 1

  name               = "${var.name}-queue-out"
  service_namespace  = aws_appautoscaling_target.this.service_namespace
  resource_id        = aws_appautoscaling_target.this.resource_id
  scalable_dimension = aws_appautoscaling_target.this.scalable_dimension
  policy_type        = "StepScaling"

  step_scaling_policy_configuration {
    adjustment_type         = "ChangeInCapacity"
    metric_aggregation_type = "Maximum"
    # Lo que tarda en arrancar una instancia desde cero: antes de eso, sumar otra tarea no
    # acelera nada.
    cooldown = 300

    step_adjustment {
      metric_interval_lower_bound = 0
      metric_interval_upper_bound = var.queue_scaling.backlog_step
      scaling_adjustment          = 1
    }

    step_adjustment {
      metric_interval_lower_bound = var.queue_scaling.backlog_step
      scaling_adjustment          = 2
    }
  }
}

resource "aws_cloudwatch_metric_alarm" "queue_out" {
  count = var.queue_scaling == null ? 0 : 1

  alarm_name          = "${var.name}-queue-out"
  alarm_description   = "Hay mensajes esperando en ${var.queue_scaling.queue_name}: suma tareas a ${var.name}"
  namespace           = "AWS/SQS"
  metric_name         = local.queue_metrics.visibles
  dimensions          = { QueueName = var.queue_scaling.queue_name }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_appautoscaling_policy.queue_out[0].arn]
}

resource "aws_appautoscaling_policy" "queue_in" {
  count = var.queue_scaling == null ? 0 : 1

  name               = "${var.name}-queue-in"
  service_namespace  = aws_appautoscaling_target.this.service_namespace
  resource_id        = aws_appautoscaling_target.this.resource_id
  scalable_dimension = aws_appautoscaling_target.this.scalable_dimension
  policy_type        = "StepScaling"

  step_scaling_policy_configuration {
    adjustment_type         = "ExactCapacity"
    metric_aggregation_type = "Maximum"
    cooldown                = 60

    step_adjustment {
      metric_interval_upper_bound = 0
      scaling_adjustment          = var.min_count
    }
  }
}

# Los demorados también son trabajo pendiente: una exportación que espera un archivado vuelve
# a la cola con demora, y sin contarlos el servicio bajaría a cero entre un intento y otro.
resource "aws_cloudwatch_metric_alarm" "queue_in" {
  count = var.queue_scaling == null ? 0 : 1

  alarm_name          = "${var.name}-queue-in"
  alarm_description   = "${var.queue_scaling.queue_name} lleva ${var.queue_scaling.idle_minutes} minutos sin mensajes: baja ${var.name} al mínimo"
  evaluation_periods  = var.queue_scaling.idle_minutes
  datapoints_to_alarm = var.queue_scaling.idle_minutes
  comparison_operator = "LessThanOrEqualToThreshold"
  threshold           = 0
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_appautoscaling_policy.queue_in[0].arn]

  metric_query {
    id          = "pendientes"
    expression  = join(" + ", keys(local.queue_metrics))
    label       = "Mensajes visibles, en curso y demorados"
    return_data = true
  }

  dynamic "metric_query" {
    for_each = local.queue_metrics
    content {
      id = metric_query.key
      metric {
        namespace   = "AWS/SQS"
        metric_name = metric_query.value
        dimensions  = { QueueName = var.queue_scaling.queue_name }
        period      = 60
        stat        = "Maximum"
      }
    }
  }
}
