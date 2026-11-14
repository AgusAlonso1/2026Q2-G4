data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

# El bucket no es un aws_s3_bucket: lo crea scripts/buckets.sh, como los otros dos (el SCP de AWS
# Academy no deja leer su object lock). Acá sólo se configura.
locals {
  account     = data.aws_caller_identity.current.account_id
  region      = data.aws_region.current.region
  bucket_name = format("%s-logs-%s-%s-an", var.name_prefix, local.account, local.region)
  bucket_arn  = "arn:aws:s3:::${local.bucket_name}"
  trail_name  = "${var.name_prefix}-trail"
  # Armado a mano y no desde el recurso: la política del bucket tiene que existir antes que el trail.
  trail_arn = "arn:aws:cloudtrail:${local.region}:${local.account}:trail/${local.trail_name}"

  prefixes = {
    cloudtrail = "cloudtrail"
    flow_logs  = "flow-logs"
    config     = "config"
  }

  # Reglas administradas por AWS. La del bucket público marca a propósito el del frontend: es el
  # único público, y que Config lo muestre es la prueba de que los demás no lo son.
  config_rules = {
    s3-sin-lectura-publica = "S3_BUCKET_PUBLIC_READ_PROHIBITED"
    s3-solo-https          = "S3_BUCKET_SSL_REQUESTS_ONLY"
    rds-cifrada            = "RDS_STORAGE_ENCRYPTED"
    rds-multi-az           = "RDS_MULTI_AZ_SUPPORT"
    rds-sin-acceso-publico = "RDS_INSTANCE_PUBLIC_ACCESS_CHECK"
    sin-ssh-abierto        = "INCOMING_SSH_DISABLED"
    discos-cifrados        = "ENCRYPTED_VOLUMES"
    vpc-con-flow-logs      = "VPC_FLOW_LOGS_ENABLED"
    cloudtrail-habilitado  = "CLOUD_TRAIL_ENABLED"
  }
}

# --- Bucket de logs ------------------------------------------------------------------------------
# Separado del de los datos: lo escriben servicios de AWS, no la aplicación, y quien lee el
# archivo del ledger no tiene por qué leer la auditoría. SSE-S3 y no la clave del proyecto: así
# CloudTrail, los flow logs y Config escriben sin permisos sobre la clave.

resource "aws_s3_bucket_public_access_block" "logs" {
  bucket = local.bucket_name

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "logs" {
  bucket = local.bucket_name

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_versioning" "logs" {
  bucket = local.bucket_name

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "logs" {
  bucket = local.bucket_name

  rule {
    id     = "retencion"
    status = "Enabled"

    filter {}

    expiration {
      days = var.log_retention_days
    }

    noncurrent_version_expiration {
      noncurrent_days = 1
    }
  }

  depends_on = [aws_s3_bucket_versioning.logs]
}

resource "aws_s3_bucket_policy" "logs" {
  bucket = local.bucket_name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "DenyInsecureTransport"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource  = [local.bucket_arn, "${local.bucket_arn}/*"]
        Condition = { Bool = { "aws:SecureTransport" = "false" } }
      },
      {
        Sid       = "CloudTrailAcl"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:GetBucketAcl"
        Resource  = local.bucket_arn
        Condition = { StringEquals = { "aws:SourceArn" = local.trail_arn } }
      },
      {
        Sid       = "CloudTrailWrite"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:PutObject"
        Resource  = "${local.bucket_arn}/${local.prefixes.cloudtrail}/AWSLogs/${local.account}/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl"  = "bucket-owner-full-control"
            "aws:SourceArn" = local.trail_arn
          }
        }
      },
      {
        Sid       = "FlowLogsAcl"
        Effect    = "Allow"
        Principal = { Service = "delivery.logs.amazonaws.com" }
        Action    = ["s3:GetBucketAcl", "s3:ListBucket"]
        Resource  = local.bucket_arn
        Condition = {
          StringEquals = { "aws:SourceAccount" = local.account }
          ArnLike      = { "aws:SourceArn" = "arn:aws:logs:${local.region}:${local.account}:*" }
        }
      },
      {
        Sid       = "FlowLogsWrite"
        Effect    = "Allow"
        Principal = { Service = "delivery.logs.amazonaws.com" }
        Action    = "s3:PutObject"
        Resource  = "${local.bucket_arn}/${local.prefixes.flow_logs}/AWSLogs/${local.account}/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl"      = "bucket-owner-full-control"
            "aws:SourceAccount" = local.account
          }
          ArnLike = { "aws:SourceArn" = "arn:aws:logs:${local.region}:${local.account}:*" }
        }
      },
      {
        Sid       = "ConfigAcl"
        Effect    = "Allow"
        Principal = { Service = "config.amazonaws.com" }
        Action    = ["s3:GetBucketAcl", "s3:ListBucket"]
        Resource  = local.bucket_arn
        Condition = { StringEquals = { "aws:SourceAccount" = local.account } }
      },
      {
        Sid       = "ConfigWrite"
        Effect    = "Allow"
        Principal = { Service = "config.amazonaws.com" }
        Action    = "s3:PutObject"
        Resource  = "${local.bucket_arn}/${local.prefixes.config}/AWSLogs/${local.account}/Config/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl"      = "bucket-owner-full-control"
            "aws:SourceAccount" = local.account
          }
        }
      },
    ]
  })

  depends_on = [aws_s3_bucket_public_access_block.logs]
}

# --- CloudTrail ----------------------------------------------------------------------------------
# Sin trail, CloudTrail guarda 90 días de eventos de gestión y nada más. El trail los deja en S3
# con la retención del bucket y un digest por hora para probar que nadie los tocó. El primero de
# eventos de gestión es gratis. AWS Academy no deja mandarlo también a CloudWatch Logs.

# AWS Academy no deja mandar el trail a CloudWatch Logs.
#tfsec:ignore:aws-cloudtrail-ensure-cloudwatch-integration
resource "aws_cloudtrail" "this" {
  name                          = local.trail_name
  kms_key_id                    = var.kms_key_arn
  s3_bucket_name                = local.bucket_name
  s3_key_prefix                 = local.prefixes.cloudtrail
  include_global_service_events = true
  is_multi_region_trail         = false #tfsec:ignore:aws-cloudtrail-enable-all-regions AWS Academy bloquea las demás regiones y la carga está toda acá
  enable_log_file_validation    = true

  depends_on = [aws_s3_bucket_policy.logs]
}

# --- VPC Flow Logs -------------------------------------------------------------------------------
# A S3 y no a CloudWatch Logs: es más barato, y la confianza de LabRole no incluye a
# vpc-flow-logs.amazonaws.com, que es quien escribe en CloudWatch. En Parquet y con particiones
# estilo Hive, como el archivo del ledger: se pueden consultar con Athena.

resource "aws_flow_log" "vpc" {
  vpc_id                   = var.vpc_id
  traffic_type             = var.flow_log_traffic_type
  log_destination_type     = "s3"
  log_destination          = "${local.bucket_arn}/${local.prefixes.flow_logs}/"
  max_aggregation_interval = 600

  destination_options {
    file_format                = "parquet"
    hive_compatible_partitions = true
    per_hour_partition         = true
  }

  tags = { Name = "${var.name_prefix}-vpc" }

  depends_on = [aws_s3_bucket_policy.logs]
}

# --- AWS Config ----------------------------------------------------------------------------------
# Registra cómo están configurados los recursos y los evalúa contra reglas: el auditor, frente a
# CloudWatch (cómo anda) y CloudTrail (quién hizo qué). LabRole no confía en config.amazonaws.com,
# así que graba con su rol vinculado al servicio, que AWS Academy sí deja crear.

# Con el provider sin etiquetas: AWS Academy no deja etiquetar el rol (iam:TagRole).
resource "aws_iam_service_linked_role" "config" {
  provider = aws.sin_tags

  aws_service_name = "config.amazonaws.com"
}

resource "aws_config_configuration_recorder" "this" {
  name     = var.name_prefix
  role_arn = aws_iam_service_linked_role.config.arn

  recording_group {
    all_supported                 = false
    include_global_resource_types = false
    resource_types                = var.config_resource_types

    recording_strategy {
      use_only = "INCLUSION_BY_RESOURCE_TYPES"
    }
  }
}

resource "aws_config_delivery_channel" "this" {
  name           = var.name_prefix
  s3_bucket_name = local.bucket_name
  s3_key_prefix  = local.prefixes.config

  snapshot_delivery_properties {
    delivery_frequency = "TwentyFour_Hours"
  }

  depends_on = [aws_config_configuration_recorder.this, aws_s3_bucket_policy.logs]
}

resource "aws_config_configuration_recorder_status" "this" {
  name       = aws_config_configuration_recorder.this.name
  is_enabled = true

  depends_on = [aws_config_delivery_channel.this]
}

resource "aws_config_config_rule" "this" {
  for_each = local.config_rules

  name = "${var.name_prefix}-${each.key}"

  source {
    owner             = "AWS"
    source_identifier = each.value
  }

  depends_on = [aws_config_configuration_recorder_status.this]
}

# --- Presupuesto ---------------------------------------------------------------------------------
# Avisa por mail al 80 % de lo gastado en el mes y cuando el pronóstico pasa el 100 %. En una
# cuenta con varios proyectos se filtraría por la etiqueta Project de default_tags.

resource "aws_budgets_budget" "this" {
  count = length(var.budget_emails) > 0 ? 1 : 0

  name         = "${var.name_prefix}-mensual"
  budget_type  = "COST"
  limit_amount = tostring(var.budget_limit_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = var.budget_emails
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = var.budget_emails
  }
}
