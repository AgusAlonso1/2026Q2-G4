locals {
  name_prefix = var.project_name
  api_url     = "http://${module.load_balancer.dns_name}"

  athena_results_prefix = "athena-results/"

  common_tags = {
    Project   = "nucleo-ledger"
    TP        = "tp3"
    ManagedBy = "terraform"
  }

  az_count = 2
  azs      = slice(data.aws_availability_zones.available.names, 0, local.az_count)

  subnet_offsets = {
    public  = 0
    app     = 10
    archive = 12
    data    = 20
  }

  subnet_cidrs = {
    for layer, offset in local.subnet_offsets :
    layer => [for i in range(local.az_count) : cidrsubnet(var.vpc_cidr, 8, offset + i)]
  }

  subnet_names = {
    for layer in keys(local.subnet_offsets) :
    layer => [for az in local.azs : "${local.name_prefix}-${layer}-${substr(az, -1, 1)}"]
  }

  # Cada parte del archivo en paralelo usa su conexión.
  archive_parallelism = 2

  exports_consumers                  = 2
  exports_max_attempts               = 3
  exports_visibility_timeout_seconds = 120

  services = {
    api = {
      command                            = ["/api"]
      instance_type                      = var.api_instance_type
      target_capacity                    = 80
      maximum_scaling_step_size          = 2
      cpu                                = 1024
      memory                             = 512
      port                               = var.app_port
      target_group_arn                   = module.load_balancer.target_group_arn
      subnet_ids                         = module.vpc.private_subnets
      security_group_id                  = module.security_groups.ids.app
      min_count                          = 2
      max_count                          = 5 # el ASG admite max_count + 1, y AWS Academy no deja más de 6
      cpu_target                         = 60
      schedule                           = {}
      queue_scaling                      = null
      db_pool                            = 5
      deployment_minimum_healthy_percent = 50
      deployment_maximum_percent         = 200
      environment = {
        APP_PORT                  = tostring(var.app_port)
        S3_BUCKET                 = module.storage.bucket_name
        AWS_REGION                = var.aws_region
        COGNITO_ISSUER            = module.auth.issuer
        COGNITO_CLIENT_ID         = module.auth.client_id
        CORS_ORIGENES             = module.website.website_url
        PROVEEDOR_URL             = coalesce(var.proveedor_url, "${local.api_url}/servicio-mock")
        SNS_TOPIC_ARN             = module.events.topic_arn
        TITULARES_TOPIC_ARN       = module.events.titulares_topic_arn
        APP_URL                   = module.website.website_url
        EXPORTACIONES_QUEUE_URL   = module.events.exports_queue_url
        EXPORTACIONES_INTENTOS    = tostring(local.exports_max_attempts)
        EXPORTACIONES_VISIBILIDAD = "${local.exports_visibility_timeout_seconds}s"
      }
    }
    archive = {
      command                            = ["/archivador"]
      instance_type                      = var.archive_instance_type
      target_capacity                    = 100
      maximum_scaling_step_size          = 1
      cpu                                = 1024
      memory                             = 3072
      port                               = null
      target_group_arn                   = null
      subnet_ids                         = module.vpc.intra_subnets
      security_group_id                  = module.security_groups.ids.archive
      min_count                          = 0
      max_count                          = 1
      cpu_target                         = null
      schedule                           = var.archive_schedule
      queue_scaling                      = null
      db_pool                            = local.archive_parallelism
      deployment_minimum_healthy_percent = 50
      deployment_maximum_percent         = 200
      environment = {
        S3_BUCKET                  = module.storage.bucket_name
        AWS_REGION                 = var.aws_region
        ARCHIVADO_FILAS_POR_PARTE  = tostring(var.archive_rows_per_part)
        ARCHIVADO_FILAS_POR_BLOQUE = tostring(var.archive_rows_per_row_group)
        ARCHIVADO_PARALELISMO      = tostring(local.archive_parallelism)
      }
    }
    exports = {
      command                   = ["/exportador"]
      instance_type             = var.exports_instance_type
      target_capacity           = 100
      maximum_scaling_step_size = 1
      cpu                       = 512
      memory                    = 1024
      port                      = null
      target_group_arn          = null
      subnet_ids                = module.vpc.intra_subnets
      security_group_id         = module.security_groups.ids.exports
      min_count                 = 0
      max_count                 = 2 # AWS Academy limita las instancias que corren a la vez en la cuenta
      cpu_target                = null
      schedule                  = {}
      queue_scaling             = { queue_name = module.events.exports_queue_name, backlog_step = 100 }
      # Cada consumidor tiene abierto el cursor de asientos mientras arma el CSV, y el latido
      # que renueva el reclamo necesita otra conexión libre: si no, espera a que termine el
      # cursor y otra tarea le puede ganar la exportación.
      db_pool = local.exports_consumers + 1
      # Sin tareas de más en un deploy: SIGTERM devuelve lo que está armando a la cola, y así
      # no hace falta lugar para otra instancia.
      deployment_minimum_healthy_percent = 0
      deployment_maximum_percent         = 100
      environment = {
        S3_BUCKET                  = module.storage.bucket_name
        AWS_REGION                 = var.aws_region
        TITULARES_TOPIC_ARN        = module.events.titulares_topic_arn
        APP_URL                    = module.website.website_url
        EXPORTACIONES_QUEUE_URL    = module.events.exports_queue_url
        EXPORTACIONES_DLQ_URL      = module.events.exports_dlq_url
        EXPORTACIONES_INTENTOS     = tostring(local.exports_max_attempts)
        EXPORTACIONES_VISIBILIDAD  = "${local.exports_visibility_timeout_seconds}s"
        EXPORTACIONES_CONSUMIDORES = tostring(local.exports_consumers)
      }
    }
  }

  # Cota para un deploy: todos los servicios a la vez en su máximo de tareas, más las que el
  # deploy suma por encima (deployment_maximum_percent).
  db_connections = sum([for s in values(local.services) : ceil(s.max_count * s.deployment_maximum_percent / 100) * s.db_pool])
  db_urls        = { for k, s in local.services : k => "${module.database.url}&pool_max_conns=${s.db_pool}" }

  # Aparte de services: son sensibles, y for_each no acepta valores sensibles.
  mock_api_key   = coalesce(var.mock_api_key, random_password.this["mock"].result)
  webhook_secret = coalesce(var.webhook_secret, random_password.this["webhook"].result)

  notification_emails = compact([for e in split(",", var.notification_emails) : trimspace(e)])

  secret_values = {
    api = {
      DB_URL            = local.db_urls.api
      ADMIN_API_KEY     = random_password.this["admin"].result
      MOCK_API_KEY      = local.mock_api_key
      PROVEEDOR_API_KEY = coalesce(var.proveedor_api_key, local.mock_api_key)
      WEBHOOK_SECRET    = local.webhook_secret
    }
    archive = {
      DB_URL = local.db_urls.archive
    }
    exports = {
      DB_URL = local.db_urls.exports
    }
  }
}
