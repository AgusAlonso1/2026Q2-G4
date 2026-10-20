# Las propiedades de aislamiento del diseño, verificadas sobre las reglas que genera la tabla
# de locals.tf: si alguien agrega una regla que las rompe, el CI falla.

mock_provider "aws" {}

variables {
  name_prefix       = "ledger"
  vpc_id            = "vpc-mock"
  app_port          = 8080
  db_port           = 5432
  s3_prefix_list_id = "pl-63a5400a"
}

run "aislamiento" {
  command = apply

  assert {
    condition     = join(",", [for k, r in aws_vpc_security_group_ingress_rule.this : k if r.cidr_ipv4 != null]) == "nlb_in_internet"
    error_message = "La única entrada desde un rango de direcciones es la del NLB desde Internet."
  }

  assert {
    condition     = alltrue([for r in concat(values(aws_vpc_security_group_ingress_rule.this), values(aws_vpc_security_group_egress_rule.this)) : r.from_port != 22])
    error_message = "Ningún grupo abre SSH."
  }

  assert {
    condition = toset([for r in aws_vpc_security_group_ingress_rule.this : r.referenced_security_group_id if r.security_group_id == aws_security_group.this["data"].id]) == toset([
      aws_security_group.this["app"].id,
      aws_security_group.this["archive"].id,
    ])
    error_message = "A la base sólo entran la API y el archivador."
  }

  assert {
    condition     = alltrue([for r in aws_vpc_security_group_ingress_rule.this : r.from_port == 5432 if r.security_group_id == aws_security_group.this["data"].id])
    error_message = "A la base sólo se entra por el puerto de PostgreSQL."
  }

  assert {
    condition     = alltrue([for r in aws_vpc_security_group_egress_rule.this : r.cidr_ipv4 == null if contains([aws_security_group.this["archive"].id, aws_security_group.this["data"].id, aws_security_group.this["host"].id], r.security_group_id)])
    error_message = "El archivador, la base y los hosts de ECS no tienen salida a Internet."
  }
}
