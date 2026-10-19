#!/usr/bin/env bash
set -euo pipefail

region="${REGION:-us-east-1}"
account=$(aws sts get-caller-identity --query Account --output text)
bucket="ledger-tfstate-$account-$region-an"

case "${1:-}" in
  create)
    if ! aws s3api head-bucket --bucket "$bucket" 2>/dev/null; then
      echo "Creando el bucket del state $bucket" >&2
      aws s3api create-bucket --bucket "$bucket" --region "$region" --bucket-namespace account-regional >/dev/null
      aws s3api put-bucket-versioning --bucket "$bucket" --versioning-configuration Status=Enabled
    fi
    echo "$bucket"
    ;;
  delete)
    aws s3api head-bucket --bucket "$bucket" 2>/dev/null || exit 0
    # Versionado: hay que borrar cada versión y cada delete marker antes que el bucket.
    while :; do
      batch=$(aws s3api list-object-versions --bucket "$bucket" --max-items 500 --output json |
        jq -cn '(input? // {}) | {Objects: [(.Versions // [])[], (.DeleteMarkers // [])[] | {Key, VersionId}], Quiet: true}')
      [ "$(jq '.Objects | length' <<<"$batch")" -eq 0 ] && break
      aws s3api delete-objects --bucket "$bucket" --delete "$batch" >/dev/null
    done
    aws s3api delete-bucket --bucket "$bucket"
    echo "Bucket del state $bucket borrado" >&2
    ;;
  *)
    echo "uso: $0 create|delete" >&2
    exit 2
    ;;
esac
