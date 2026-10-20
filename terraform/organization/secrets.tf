resource "random_password" "this" {
  for_each = toset(["db", "admin", "mock", "webhook"])

  length  = 32
  special = false
}
