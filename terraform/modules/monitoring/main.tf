data "aws_region" "current" {}

data "aws_caller_identity" "current" {}

locals {
  region = data.aws_region.current.region

  # Métricas que salen del log de la API: cada request deja una línea JSON con msg = "request".
  # /health no cuenta: el NLB lo pide cada pocos segundos y bajaría la latencia y subiría los pedidos.
  api_namespace = "${var.name_prefix}/api"

  metric_filters = {
    pedidos = {
      metric  = "Pedidos"
      pattern = "{ $.msg = \"request\" && $.ruta != \"/health\" }"
      value   = "1"
      default = null
      unit    = "Count"
    }
    latencia = {
      metric  = "Latencia"
      pattern = "{ $.msg = \"request\" && $.ruta != \"/health\" }"
      value   = "$.ms"
      default = null
      unit    = "Milliseconds"
    }
    errores-5xx = {
      metric  = "Respuestas5xx"
      pattern = "{ $.msg = \"request\" && $.estado >= 500 }"
      value   = "1"
      default = "0"
      unit    = "Count"
    }
  }

  db_dimensions  = { DBInstanceIdentifier = var.db_instance_identifier }
  nlb_dimensions = { LoadBalancer = var.nlb_arn_suffix, TargetGroup = var.target_group_arn_suffix }

  # Todas con los mismos atributos (null donde no aplica), para que sea un map(object) y entre en for_each.
  alarms = {
    api-5xx = {
      description        = "La API respondió ${var.errors_5xx_threshold} o más errores 5xx en 5 minutos: un error del servidor, no del pedido."
      namespace          = local.api_namespace
      metric             = "Respuestas5xx"
      dimensions         = {}
      statistic          = "Sum"
      extended_statistic = null
      period             = 300
      periods            = 1
      comparison         = "GreaterThanOrEqualToThreshold"
      threshold          = var.errors_5xx_threshold
    }
    api-latencia = {
      description        = "La latencia p99 de la API pasó ${var.latency_p99_threshold_ms} ms tres minutos seguidos."
      namespace          = local.api_namespace
      metric             = "Latencia"
      dimensions         = {}
      statistic          = null
      extended_statistic = "p99"
      period             = 60
      periods            = 3
      comparison         = "GreaterThanThreshold"
      threshold          = var.latency_p99_threshold_ms
    }
    nlb-targets-sanos = {
      description        = "El NLB tiene menos de ${var.api_min_tasks} tareas sanas de la API: atiende por debajo del mínimo."
      namespace          = "AWS/NetworkELB"
      metric             = "HealthyHostCount"
      dimensions         = local.nlb_dimensions
      statistic          = "Minimum"
      extended_statistic = null
      period             = 60
      periods            = 2
      comparison         = "LessThanThreshold"
      threshold          = var.api_min_tasks
    }
    rds-cpu = {
      description        = "La CPU de RDS estuvo por encima de ${var.db_cpu_threshold} % durante 10 minutos."
      namespace          = "AWS/RDS"
      metric             = "CPUUtilization"
      dimensions         = local.db_dimensions
      statistic          = "Average"
      extended_statistic = null
      period             = 300
      periods            = 2
      comparison         = "GreaterThanThreshold"
      threshold          = var.db_cpu_threshold
    }
    rds-conexiones = {
      description        = "Hay más de ${var.db_connections_threshold} conexiones abiertas a RDS: más de las que reparte el diseño."
      namespace          = "AWS/RDS"
      metric             = "DatabaseConnections"
      dimensions         = local.db_dimensions
      statistic          = "Maximum"
      extended_statistic = null
      period             = 60
      periods            = 3
      comparison         = "GreaterThanThreshold"
      threshold          = var.db_connections_threshold
    }
    rds-memoria = {
      description        = "RDS tiene menos de ${var.db_free_memory_threshold_mb} MB de memoria libre desde hace 10 minutos."
      namespace          = "AWS/RDS"
      metric             = "FreeableMemory"
      dimensions         = local.db_dimensions
      statistic          = "Minimum"
      extended_statistic = null
      period             = 300
      periods            = 2
      comparison         = "LessThanThreshold"
      threshold          = var.db_free_memory_threshold_mb * 1024 * 1024
    }
    rds-disco = {
      description        = "RDS tiene menos de ${var.db_free_storage_threshold_gb} GB libres en disco, que es fijo: si se llena, la base deja de escribir."
      namespace          = "AWS/RDS"
      metric             = "FreeStorageSpace"
      dimensions         = local.db_dimensions
      statistic          = "Minimum"
      extended_statistic = null
      period             = 300
      periods            = 1
      comparison         = "LessThanThreshold"
      threshold          = var.db_free_storage_threshold_gb * 1024 * 1024 * 1024
    }
  }

  # La alarma de la DLQ la crea el módulo events; acá sólo se muestra en el panel de alarmas.
  dlq_alarm_arn = "arn:aws:cloudwatch:${local.region}:${data.aws_caller_identity.current.account_id}:alarm:${var.name_prefix}-webhook-dlq"

  queries = {
    api-pedidos-lentos = {
      log_groups = [var.api_log_group_name]
      query      = <<-QUERY
        fields @timestamp, metodo, ruta, estado, ms
        | filter msg = "request" and ruta != "/health"
        | sort ms desc
        | limit 50
      QUERY
    }
    api-latencia-por-ruta = {
      log_groups = [var.api_log_group_name]
      query      = <<-QUERY
        filter msg = "request" and ruta != "/health"
        | stats count(*) as pedidos, avg(ms) as promedio_ms, pct(ms, 99) as p99_ms by metodo, ruta
        | sort pedidos desc
      QUERY
    }
    api-errores-5xx = {
      log_groups = [var.api_log_group_name]
      query      = <<-QUERY
        fields @timestamp, metodo, ruta, estado, ms
        | filter msg = "request" and estado >= 500
        | sort @timestamp desc
        | limit 100
      QUERY
    }
    errores-de-los-binarios = {
      log_groups = [var.api_log_group_name, var.archive_log_group_name]
      query      = <<-QUERY
        fields @timestamp, @log, msg, error
        | filter level = "ERROR"
        | sort @timestamp desc
        | limit 100
      QUERY
    }
    webhook-no-entregados = {
      log_groups = [var.webhook_log_group_name]
      query      = <<-QUERY
        fields @timestamp, mensaje, intento, error
        | filter msg = "webhook no entregado"
        | sort @timestamp desc
        | limit 100
      QUERY
    }
  }

  # Paneles del dashboard, en una grilla de 24 columnas: de arriba hacia abajo, el cómputo, la
  # entrada, la base y lo asíncrono. lines son las líneas de referencia (objetivos y umbrales).
  ecs         = ["ClusterName", var.cluster_name]
  api_svc     = concat(local.ecs, ["ServiceName", var.api_service_name])
  archive_svc = concat(local.ecs, ["ServiceName", var.archive_service_name])
  nlb_dims    = ["TargetGroup", var.target_group_arn_suffix, "LoadBalancer", var.nlb_arn_suffix]
  db_dims     = ["DBInstanceIdentifier", var.db_instance_identifier]

  panels = [
    {
      title = "Tareas de ECS", x = 0, y = 4, w = 8, stat = "Average", lines = []
      metrics = [
        concat(["ECS/ContainerInsights", "RunningTaskCount"], local.api_svc, [{ label = "API corriendo" }]),
        concat(["ECS/ContainerInsights", "DesiredTaskCount"], local.api_svc, [{ label = "API deseadas" }]),
        concat(["ECS/ContainerInsights", "RunningTaskCount"], local.archive_svc, [{ label = "Archivador" }]),
      ]
    },
    {
      title = "CPU de la API (%)", x = 8, y = 4, w = 8, stat = "Average"
      lines = [{ value = var.api_cpu_target, label = "objetivo del autoscaling" }]
      metrics = [
        concat(["AWS/ECS", "CPUUtilization"], local.api_svc, [{ label = "promedio" }]),
        concat(["AWS/ECS", "CPUUtilization"], local.api_svc, [{ label = "máximo", stat = "Maximum" }]),
      ]
    },
    {
      title = "Instancias del cluster", x = 16, y = 4, w = 8, stat = "Average", lines = []
      metrics = [
        concat(["ECS/ContainerInsights", "ContainerInstanceCount"], local.ecs, [{ label = "instancias registradas" }]),
      ]
    },
    {
      title = "Targets del NLB", x = 0, y = 10, w = 8, stat = "Minimum"
      lines = [{ value = var.api_min_tasks, label = "mínimo de tareas" }]
      metrics = [
        concat(["AWS/NetworkELB", "HealthyHostCount"], local.nlb_dims, [{ label = "sanos" }]),
        concat(["AWS/NetworkELB", "UnHealthyHostCount"], local.nlb_dims, [{ label = "no sanos", stat = "Maximum" }]),
      ]
    },
    {
      title = "Latencia de la API (ms)", x = 8, y = 10, w = 8, stat = "p50"
      lines = [{ value = var.latency_p99_threshold_ms, label = "umbral p99" }]
      metrics = [
        [local.api_namespace, "Latencia", { label = "p50" }],
        [local.api_namespace, "Latencia", { label = "p99", stat = "p99" }],
      ]
    },
    {
      title = "Pedidos y 5xx por minuto", x = 16, y = 10, w = 8, stat = "Sum", lines = []
      metrics = [
        [local.api_namespace, "Pedidos", { label = "pedidos" }],
        [local.api_namespace, "Respuestas5xx", { label = "5xx" }],
      ]
    },
    {
      title = "CPU de RDS (%)", x = 0, y = 16, w = 6, stat = "Average"
      lines = [{ value = var.db_cpu_threshold, label = "umbral" }]
      metrics = [
        concat(["AWS/RDS", "CPUUtilization"], local.db_dims),
      ]
    },
    {
      title = "Conexiones a RDS", x = 6, y = 16, w = 6, stat = "Maximum"
      lines = [{ value = var.db_connections_threshold, label = "umbral" }]
      metrics = [
        concat(["AWS/RDS", "DatabaseConnections"], local.db_dims),
      ]
    },
    {
      title = "Memoria libre de RDS (bytes)", x = 12, y = 16, w = 6, stat = "Minimum"
      lines = [{ value = var.db_free_memory_threshold_mb * 1024 * 1024, label = "umbral" }]
      metrics = [
        concat(["AWS/RDS", "FreeableMemory"], local.db_dims),
      ]
    },
    {
      title = "Disco libre de RDS (bytes)", x = 18, y = 16, w = 6, stat = "Minimum"
      lines = [{ value = var.db_free_storage_threshold_gb * 1024 * 1024 * 1024, label = "umbral" }]
      metrics = [
        concat(["AWS/RDS", "FreeStorageSpace"], local.db_dims),
      ]
    },
    {
      title = "Cola del webhook", x = 0, y = 22, w = 8, stat = "Maximum", lines = []
      metrics = [
        ["AWS/SQS", "ApproximateNumberOfMessagesVisible", "QueueName", var.webhook_queue_name, { label = "en la cola" }],
        ["AWS/SQS", "ApproximateNumberOfMessagesVisible", "QueueName", var.webhook_dlq_name, { label = "en la DLQ" }],
      ]
    },
    {
      title = "Lambda del webhook", x = 8, y = 22, w = 8, stat = "Sum", lines = []
      metrics = [
        ["AWS/Lambda", "Invocations", "FunctionName", var.webhook_function_name, { label = "invocaciones" }],
        ["AWS/Lambda", "Errors", "FunctionName", var.webhook_function_name, { label = "errores" }],
      ]
    },
    {
      title = "WAF del login", x = 16, y = 22, w = 8, stat = "Sum", lines = []
      metrics = [
        ["AWS/WAFV2", "AllowedRequests", "WebACL", var.waf_web_acl_name, "Rule", "ALL", "Region", local.region, { label = "permitidos" }],
        ["AWS/WAFV2", "BlockedRequests", "WebACL", var.waf_web_acl_name, "Rule", "ALL", "Region", local.region, { label = "bloqueados" }],
      ]
    },
  ]
}

resource "aws_cloudwatch_log_metric_filter" "api" {
  for_each = local.metric_filters

  name           = "${var.name_prefix}-api-${each.key}"
  log_group_name = var.api_log_group_name
  pattern        = each.value.pattern

  metric_transformation {
    namespace     = local.api_namespace
    name          = each.value.metric
    value         = each.value.value
    default_value = each.value.default
    unit          = each.value.unit
  }
}

resource "aws_cloudwatch_metric_alarm" "this" {
  for_each = local.alarms

  alarm_name          = "${var.name_prefix}-${each.key}"
  alarm_description   = each.value.description
  namespace           = each.value.namespace
  metric_name         = each.value.metric
  dimensions          = each.value.dimensions
  statistic           = each.value.statistic
  extended_statistic  = each.value.extended_statistic
  period              = each.value.period
  evaluation_periods  = each.value.periods
  comparison_operator = each.value.comparison
  threshold           = each.value.threshold
  # Sin datos no es una falla: sin tráfico no hay latencia ni 5xx, y la base apagada no es una alarma de capacidad.
  treat_missing_data = "notBreaching"
  alarm_actions      = [var.alarm_topic_arn]
  ok_actions         = [var.alarm_topic_arn]

  # Las métricas de la API existen recién cuando el filtro publica el primer dato.
  depends_on = [aws_cloudwatch_log_metric_filter.api]
}

resource "aws_cloudwatch_dashboard" "this" {
  dashboard_name = var.name_prefix

  dashboard_body = jsonencode({
    widgets = concat(
      [
        {
          type   = "text"
          x      = 0
          y      = 0
          width  = 24
          height = 1
          properties = {
            markdown = "**${var.name_prefix}** · cómputo, entrada, base y eventos. Las alarmas avisan por mail al topic de alertas, al dispararse y al volver a OK."
          }
        },
        {
          type   = "alarm"
          x      = 0
          y      = 1
          width  = 24
          height = 3
          properties = {
            title  = "Alarmas"
            alarms = concat([for a in aws_cloudwatch_metric_alarm.this : a.arn], [local.dlq_alarm_arn])
          }
        },
      ],
      [
        for p in local.panels : {
          type   = "metric"
          x      = p.x
          y      = p.y
          width  = p.w
          height = 6
          properties = {
            title       = p.title
            region      = local.region
            view        = "timeSeries"
            stacked     = false
            period      = 60
            stat        = p.stat
            metrics     = p.metrics
            annotations = { horizontal = p.lines }
            yAxis       = { left = { min = 0 } }
          }
        }
      ],
    )
  })
}

resource "aws_cloudwatch_query_definition" "this" {
  for_each = local.queries

  name            = "${var.name_prefix}/${each.key}"
  log_group_names = each.value.log_groups
  query_string    = each.value.query
}
