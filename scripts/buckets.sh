#!/usr/bin/env bash
set -euo pipefail

region="${REGION:-us-east-1}"
# El mismo prefijo que project_name: el Makefile exporta PROJECT y TF_VAR_project_name juntos.
name="${PROJECT:-ledger}"
account=$(aws sts get-caller-identity --query Account --output text)
suffix="$account-$region-an"
state="$name-tfstate-$suffix"
# Los buckets del proyecto se crean acá y no con aws_s3_bucket: el SCP de AWS Academy niega
# s3:GetBucketObjectLockConfiguration, y el provider lo lee cada vez que refresca un bucket.
# Son los nombres que arman modules/storage, modules/website y modules/audit.
# El de logs lo configura modules/audit: ahí escribe CloudTrail.
project=("$name-$suffix" "$name-web-$suffix" "$name-logs-$suffix")

# 0 si el bucket existe, 1 si no existe. Cualquier otro error (credenciales vencidas, permisos)
# corta el script: antes se tomaba como "no existe", y un destroy con la sesión del laboratorio
# vencida terminaba bien dejando los buckets.
exists() {
  local salida
  if salida=$(aws s3api head-bucket --bucket "$1" 2>&1 >/dev/null); then
    return 0
  fi
  if grep -q -E '\(404\)|Not Found' <<<"$salida"; then
    return 1
  fi
  echo "No se pudo consultar el bucket $1: $salida" >&2
  exit 1
}

create() {
  echo "Creando el bucket $1" >&2
  aws s3api create-bucket --bucket "$1" --region "$region" --bucket-namespace account-regional >/dev/null
}

delete() {
  exists "$1" || return 0
  # Versionado: hay que borrar cada versión y cada delete marker antes que el bucket.
  while :; do
    batch=$(aws s3api list-object-versions --bucket "$1" --max-items 500 --output json |
      jq -cn '(input? // {}) | {Objects: [(.Versions // [])[], (.DeleteMarkers // [])[] | {Key, VersionId}], Quiet: true}')
    [ "$(jq '.Objects | length' <<<"$batch")" -eq 0 ] && break
    aws s3api delete-objects --bucket "$1" --delete "$batch" >/dev/null
  done
  aws s3api delete-bucket --bucket "$1"
  echo "Bucket $1 borrado" >&2
}

case "${1:-}" in
  state)
    if ! exists "$state"; then
      create "$state"
      aws s3api put-bucket-versioning --bucket "$state" --versioning-configuration Status=Enabled
    fi
    # Como los demás buckets, sólo por HTTPS: el state tiene los secretos que genera Terraform.
    # Se aplica siempre (es idempotente) para que también la tengan los buckets creados antes.
    aws s3api put-bucket-policy --bucket "$state" --policy "$(jq -cn --arg b "arn:aws:s3:::$state" \
      '{Version: "2012-10-17", Statement: [{Sid: "DenyInsecureTransport", Effect: "Deny", Principal: "*",
        Action: "s3:*", Resource: [$b, "\($b)/*"], Condition: {Bool: {"aws:SecureTransport": "false"}}}]}')"
    echo "$state"
    ;;
  create)
    for bucket in "${project[@]}"; do
      exists "$bucket" || create "$bucket"
    done
    ;;
  delete)
    for bucket in "${project[@]}" "$state"; do
      delete "$bucket"
    done
    ;;
  *)
    echo "uso: $0 state|create|delete" >&2
    exit 2
    ;;
esac
