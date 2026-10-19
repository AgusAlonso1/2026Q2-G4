resource "random_password" "db" {
  length  = 32
  special = false
}

# TODO: random_password para ADMIN_API_KEY (no se comparte con nadie). JWT_SECRET ya no hace
# falta: las sesiones las emite Cognito y la API las valida contra el JWKS del pool.
# MOCK_API_KEY y WEBHOOK_SECRET NO van acá: se comparten con el otro equipo, así que
# entran por variable sensible (ver variables.tf).
