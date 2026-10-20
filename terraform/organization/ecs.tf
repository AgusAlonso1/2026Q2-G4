resource "aws_ecr_repository" "ledger" {
  name                 = local.name_prefix
  image_tag_mutability = "IMMUTABLE"
  force_delete         = true

  # Escaneo básico: cada push se revisa contra la base de CVE, sin costo. Con tags
  # inmutables cada imagen se sube una sola vez, así que cada versión queda escaneada.
  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecs_cluster" "this" {
  name = "${local.name_prefix}-cluster"
}

resource "aws_ecs_cluster_capacity_providers" "this" {
  cluster_name       = aws_ecs_cluster.this.name
  capacity_providers = [for c in module.capacity : c.capacity_provider_name]
}
