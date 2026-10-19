output "bucket_name" {
  description = "Nombre del bucket"
  value       = aws_s3_bucket.this.id
}

output "bucket_arn" {
  description = "ARN del bucket"
  value       = aws_s3_bucket.this.arn
}

output "artifact_key" {
  description = "Clave del tar.gz de binarios dentro del bucket"
  value       = aws_s3_object.artifact.key
}
