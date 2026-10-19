locals {
  name_prefix = var.project_name
  api_url     = "http://${module.load_balancer.dns_name}"

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

  services = {
    api = {
      subnet_ids       = module.vpc.private_subnets
      min_size         = 2
      desired_capacity = 2
      max_size         = 6
    }
    archive = {
      subnet_ids       = module.vpc.intra_subnets
      min_size         = 0
      desired_capacity = 0
      max_size         = 1
    }
  }
}
