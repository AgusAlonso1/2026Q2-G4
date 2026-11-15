# audit

Auditoría y costos. Configura el bucket de logs, `<prefijo>-logs-<cuenta>-<región>-an`, que crea
`scripts/buckets.sh` (el SCP de AWS Academy no deja que sea un `aws_s3_bucket`), con SSE-S3,
versionado, bloqueo público, sólo HTTPS y una retención de `log_retention_days`.

- **CloudTrail** (`cloudtrail/`): quién hizo qué en la cuenta, cifrado con la clave del proyecto y
  con validación de integridad. Regional, porque AWS Academy bloquea las demás regiones, y sin
  CloudWatch Logs, que el laboratorio no deja.
- **AWS Budgets**: un presupuesto mensual que avisa por mail al 80 % de lo gastado y cuando el
  pronóstico pasa el 100 %. Sin emails no se crea.

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.9 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.28 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.28 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [aws_budgets_budget.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/budgets_budget) | resource |
| [aws_cloudtrail.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudtrail) | resource |
| [aws_s3_bucket_lifecycle_configuration.logs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration) | resource |
| [aws_s3_bucket_policy.logs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_policy) | resource |
| [aws_s3_bucket_public_access_block.logs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_public_access_block) | resource |
| [aws_s3_bucket_server_side_encryption_configuration.logs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_server_side_encryption_configuration) | resource |
| [aws_s3_bucket_versioning.logs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_versioning) | resource |
| [aws_caller_identity.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/caller_identity) | data source |
| [aws_region.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/region) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_budget_emails"></a> [budget\_emails](#input\_budget\_emails) | Direcciones que reciben los avisos del presupuesto. Vacío: no se crea el presupuesto. | `list(string)` | `[]` | no |
| <a name="input_budget_limit_usd"></a> [budget\_limit\_usd](#input\_budget\_limit\_usd) | Presupuesto mensual en dólares. El crédito del laboratorio es de 50 para toda la materia. | `number` | `50` | no |
| <a name="input_kms_key_arn"></a> [kms\_key\_arn](#input\_kms\_key\_arn) | Clave del proyecto con la que se cifran los logs del trail. Su política tiene que dejar cifrar a CloudTrail. | `string` | n/a | yes |
| <a name="input_log_retention_days"></a> [log\_retention\_days](#input\_log\_retention\_days) | Días que se guardan los logs del trail en el bucket de logs. | `number` | `90` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefijo del bucket de logs, el trail y el presupuesto. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | Bucket de logs del trail. |
| <a name="output_budget_name"></a> [budget\_name](#output\_budget\_name) | Presupuesto mensual, o null si no hay direcciones para avisar. |
| <a name="output_trail_arn"></a> [trail\_arn](#output\_trail\_arn) | ARN del trail de CloudTrail. |
<!-- END_TF_DOCS -->
