#!/usr/bin/env bash
# Genera carga contra la API para ver la elasticidad en el dashboard de CloudWatch: sube la CPU,
# después las tareas deseadas y por último las instancias.
#
#   scripts/carga.sh [segundos] [concurrencia]       (o make carga SEGUNDOS=... CONCURRENCIA=...)
#
# Con TOKEN (el access token de una sesión del sitio) pide los movimientos de la cuenta de esa
# sesión: valida el JWT y consulta la base en cada pedido, como un usuario real. Sin TOKEN pide la
# raíz de la API, que no toca la base: alcanza para ver los pedidos y la latencia, no para escalar.
# Para sacar el token del sitio, en la consola del navegador:
#   JSON.parse(sessionStorage.getItem('nucleo.sesion')).token
#
# Usa hey (https://github.com/rakyll/hey): brew install hey. Desde CloudShell, en la misma región
# que la API, la latencia es menor y la misma concurrencia genera más pedidos:
#   curl -sSLo hey https://hey-release.s3.us-east-2.amazonaws.com/hey_linux_amd64 && chmod +x hey
set -euo pipefail

segundos="${1:-300}"
concurrencia="${2:-200}"
api="${API_URL:-$(terraform -chdir="$(dirname "$0")/../terraform/organization" output -raw api_url)}"
hey="${HEY:-hey}"

command -v "$hey" >/dev/null || { echo "Falta hey: brew install hey (o HEY=./hey en CloudShell)" >&2; exit 1; }

if [ -n "${TOKEN:-}" ]; then
  sesion=$(curl -fsS -H "Authorization: Bearer $TOKEN" "$api/sesiones/actual")
  cuenta=$(jq -r '.cuenta_id // empty' <<<"$sesion")
  [ -n "$cuenta" ] || { echo "El token no tiene una cuenta: entrá al sitio y creala primero" >&2; exit 1; }
  url="$api/cuentas/$cuenta/movimientos"
  echo "Pidiendo $url con $concurrencia conexiones durante $segundos s" >&2
  exec "$hey" -z "${segundos}s" -c "$concurrencia" -H "Authorization: Bearer $TOKEN" "$url"
fi

echo "Sin TOKEN: pidiendo $api/ con $concurrencia conexiones durante $segundos s" >&2
exec "$hey" -z "${segundos}s" -c "$concurrencia" "$api/"
