resource "random_password" "this" {
  for_each = toset(["db", "admin", "mock", "webhook"])

  length  = 32
  special = false
}

# Un secreto por servicio, con sólo lo que ese servicio usa. ECS los lee al arrancar cada
# tarea y los pasa como variables de entorno: en la task definition queda el ARN, no el valor.
resource "aws_secretsmanager_secret" "this" {
  for_each = local.services

  name        = "${local.name_prefix}/${each.key}"
  description = "Variables secretas de ${local.name_prefix}-${each.key}"

  # Sin ventana de recuperación: después de un destroy el nombre queda libre para el próximo deploy.
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "this" {
  for_each = local.services

  secret_id     = aws_secretsmanager_secret.this[each.key].id
  secret_string = jsonencode(local.secret_values[each.key])
}
