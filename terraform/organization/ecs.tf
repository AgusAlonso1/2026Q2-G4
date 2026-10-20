resource "aws_ecr_repository" "ledger" {
  name                 = local.name_prefix
  image_tag_mutability = "IMMUTABLE"
  force_delete         = true
}

resource "aws_ecs_cluster" "this" {
  name = "${local.name_prefix}-cluster"
}

resource "aws_ecs_cluster_capacity_providers" "this" {
  cluster_name       = aws_ecs_cluster.this.name
  capacity_providers = [for c in module.capacity : c.capacity_provider_name]
}
