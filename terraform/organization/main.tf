module "security_groups" {
  source = "../modules/security_groups"

  name_prefix       = local.name_prefix
  vpc_id            = module.vpc.vpc_id
  app_port          = var.app_port
  db_port           = var.db_port
  s3_prefix_list_id = aws_vpc_endpoint.s3.prefix_list_id
}

module "storage" {
  source = "../modules/storage"

  name_prefix = local.name_prefix
}

module "website" {
  source = "../modules/website"

  name_prefix = "${local.name_prefix}-web"
  site_dir    = "${path.root}/../../apps/web"

  runtime_config = {
    api_url    = local.api_url
    login_url  = module.auth.login_url
    logout_url = module.auth.logout_url
  }
}

module "auth" {
  source = "../modules/auth"

  name_prefix         = local.name_prefix
  frontend_url        = module.website.website_url
  lambda_role_arn     = data.aws_iam_role.lab.arn
  callback_source_dir = "${path.root}/../../apps/callback"
}

module "database" {
  source = "../modules/database"

  name_prefix          = local.name_prefix
  db_subnet_group_name = module.vpc.database_subnet_group_name
  security_group_id    = module.security_groups.ids.data
  password             = random_password.db.result
  port                 = var.db_port
  engine_version       = var.db_engine_version
  instance_class       = var.db_instance_class
  multi_az             = var.db_multi_az
}

module "load_balancer" {
  source = "../modules/load_balancer"

  name_prefix       = local.name_prefix
  vpc_id            = module.vpc.vpc_id
  public_subnet_ids = module.vpc.public_subnets
  security_group_id = module.security_groups.ids.nlb
  app_port          = var.app_port
  health_check_path = "/health"
}

module "service" {
  source   = "../modules/service"
  for_each = local.services

  name                 = "${local.name_prefix}-${each.key}"
  subnet_ids           = each.value.subnet_ids
  ami_id               = data.aws_ami.al2023.id
  instance_type        = var.instance_type
  instance_profile_arn = data.aws_iam_instance_profile.lab.arn
  min_size             = each.value.min_size
  desired_capacity     = each.value.desired_capacity
  max_size             = each.value.max_size
  # TODO: definir la capa de compute (EC2, ECS/Fargate, Lambda o EKS). Si queda EC2, falta:
  #       user_data =templatefile("${path.module}/templates/user_data.sh.tftpl", {...}),
  #       security_group_ids (api → ids.app, archive → ids.archive),
  #       target_group_arns (api → module.load_balancer.target_group_arn), health_check_type,
  #       COGNITO_ISSUER y COGNITO_CLIENT_ID en el env de la API (module.auth.issuer / client_id),
  #       política de escalado (api) y acciones programadas (archive)
}
