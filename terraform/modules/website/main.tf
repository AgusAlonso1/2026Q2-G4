data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

locals {
  site_files = {
    for path in fileset(var.site_dir, "**") :
    path => var.content_types[lower(regex("[^.]*$", path))]
    if contains(keys(var.content_types), lower(regex("[^.]*$", path))) && path != "config.json"
  }
}

resource "aws_s3_bucket" "this" {
  bucket           = format("%s-%s-%s-an", var.name_prefix, data.aws_caller_identity.current.account_id, data.aws_region.current.region)
  bucket_namespace = "account-regional"

  force_destroy = true
}

resource "aws_s3_bucket_website_configuration" "this" {
  bucket = aws_s3_bucket.this.id

  index_document {
    suffix = var.index_document
  }
}

resource "aws_s3_bucket_public_access_block" "this" {
  bucket = aws_s3_bucket.this.id

  block_public_acls       = true
  ignore_public_acls      = true
  block_public_policy     = false #tfsec:ignore:aws-s3-block-public-policy
  restrict_public_buckets = false #tfsec:ignore:aws-s3-no-public-buckets
}

# AES256 y no KMS: los usuarios anónimos del sitio no pueden tener permiso sobre una clave.
#tfsec:ignore:aws-s3-encryption-customer-key
resource "aws_s3_bucket_server_side_encryption_configuration" "this" {
  bucket = aws_s3_bucket.this.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_policy" "public_read" {
  bucket = aws_s3_bucket.this.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "PublicRead"
      Effect    = "Allow"
      Principal = "*"
      Action    = "s3:GetObject"
      Resource  = "${aws_s3_bucket.this.arn}/*"
    }]
  })

  depends_on = [aws_s3_bucket_public_access_block.this]
}

resource "aws_s3_object" "site" {
  for_each = local.site_files

  bucket       = aws_s3_bucket.this.id
  key          = each.key
  source       = "${var.site_dir}/${each.key}"
  source_hash  = filemd5("${var.site_dir}/${each.key}")
  content_type = each.value
}

# Lo que el frontend no puede saber hasta el apply (URL de la API, Hosted UI, ...). Se genera
# acá y no en site_dir: si existiera también allá, dos recursos pisarían el mismo objeto.
resource "aws_s3_object" "config" {
  count = length(var.runtime_config) > 0 ? 1 : 0

  bucket        = aws_s3_bucket.this.id
  key           = "config.json"
  content       = jsonencode(var.runtime_config)
  content_type  = var.content_types["json"]
  cache_control = "no-cache"
}
