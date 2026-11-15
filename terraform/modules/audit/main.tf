data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

# El bucket no es un aws_s3_bucket: lo crea scripts/buckets.sh, como los otros (el SCP de AWS
# Academy no deja leer su object lock). Acá sólo se configura.
locals {
  account     = data.aws_caller_identity.current.account_id
  region      = data.aws_region.current.region
  bucket_name = format("%s-logs-%s-%s-an", var.name_prefix, local.account, local.region)
  bucket_arn  = "arn:aws:s3:::${local.bucket_name}"
  trail_name  = "${var.name_prefix}-trail"
  trail_arn   = "arn:aws:cloudtrail:${local.region}:${local.account}:trail/${local.trail_name}"
  prefix      = "cloudtrail"
}

# --- Bucket de logs ------------------------------------------------------------------------------
# Separado del de los datos: lo escribe CloudTrail y no la aplicación, y quien lee el archivo del
# ledger no tiene por qué leer la auditoría. Los objetos van cifrados con la clave del proyecto
# (los cifra CloudTrail); el bucket, con SSE-S3 por defecto.

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
        Resource  = "${local.bucket_arn}/${local.prefix}/AWSLogs/${local.account}/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl"  = "bucket-owner-full-control"
            "aws:SourceArn" = local.trail_arn
          }
        }
      },
    ]
  })

  depends_on = [aws_s3_bucket_public_access_block.logs]
}

# --- CloudTrail ----------------------------------------------------------------------------------
# Sin trail, CloudTrail guarda 90 días de eventos de gestión y nada más. El trail los deja en S3
# con la retención del bucket, cifrados con la clave del proyecto y con un digest por hora para
# probar que nadie los tocó. El primero de eventos de gestión es gratis.

# AWS Academy no deja mandar el trail a CloudWatch Logs.
#tfsec:ignore:aws-cloudtrail-ensure-cloudwatch-integration
resource "aws_cloudtrail" "this" {
  name                          = local.trail_name
  kms_key_id                    = var.kms_key_arn
  s3_bucket_name                = local.bucket_name
  s3_key_prefix                 = local.prefix
  include_global_service_events = true
  is_multi_region_trail         = false #tfsec:ignore:aws-cloudtrail-enable-all-regions AWS Academy bloquea las demás regiones y la carga está toda acá
  enable_log_file_validation    = true

  depends_on = [aws_s3_bucket_policy.logs]
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
