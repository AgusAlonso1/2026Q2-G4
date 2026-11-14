# audit

Auditoría y gobernanza: los otros dos servicios de monitoreo, además de CloudWatch. Todo escribe
en un bucket de logs aparte, `<prefijo>-logs-<cuenta>-<región>-an`, que crea
`scripts/buckets.sh` (el SCP de AWS Academy no deja que sea un `aws_s3_bucket`); este módulo lo
configura con SSE-S3, versionado, bloqueo público y una política que deja escribir sólo a cada
servicio en su prefijo.

- **CloudTrail** (`cloudtrail/`): quién hizo qué en la cuenta, cifrado con la clave del proyecto
  y con validación de integridad. AWS Academy no deja mandarlo a CloudWatch Logs.
- **VPC Flow Logs** (`flow-logs/`): el tráfico de la VPC, en Parquet con particiones estilo Hive.
- **AWS Config** (`config/`): registra buckets, la base, security groups, la VPC y los discos, y
  los evalúa contra nueve reglas administradas. Graba con su rol vinculado al servicio.
- **AWS Budgets**: un presupuesto mensual que avisa por mail al 80 % y al pronóstico del 100 %.

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
| [aws_config_config_rule.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/config_config_rule) | resource |
| [aws_config_configuration_recorder.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/config_configuration_recorder) | resource |
| [aws_config_configuration_recorder_status.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/config_configuration_recorder_status) | resource |
| [aws_config_delivery_channel.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/config_delivery_channel) | resource |
| [aws_flow_log.vpc](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/flow_log) | resource |
| [aws_iam_service_linked_role.config](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_service_linked_role) | resource |
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
| <a name="input_budget_limit_usd"></a> [budget\_limit\_usd](#input\_budget\_limit\_usd) | Presupuesto mensual en dólares. El del laboratorio es de 50 para toda la materia. | `number` | `50` | no |
| <a name="input_config_resource_types"></a> [config\_resource\_types](#input\_config\_resource\_types) | Tipos de recurso que registra AWS Config. Sólo los que evalúan las reglas: Config cobra por cada cambio que registra. | `list(string)` | <pre>[<br/>  "AWS::S3::Bucket",<br/>  "AWS::RDS::DBInstance",<br/>  "AWS::EC2::SecurityGroup",<br/>  "AWS::EC2::VPC",<br/>  "AWS::EC2::Volume"<br/>]</pre> | no |
| <a name="input_flow_log_traffic_type"></a> [flow\_log\_traffic\_type](#input\_flow\_log\_traffic\_type) | Qué registra el flow log: ACCEPT, REJECT o ALL. | `string` | `"ALL"` | no |
| <a name="input_kms_key_arn"></a> [kms\_key\_arn](#input\_kms\_key\_arn) | Clave del proyecto con la que se cifran los logs del trail. Su política tiene que dejar cifrar a CloudTrail. | `string` | n/a | yes |
| <a name="input_log_retention_days"></a> [log\_retention\_days](#input\_log\_retention\_days) | Días que se guardan los logs de auditoría (CloudTrail, flow logs y Config) en el bucket de logs. | `number` | `90` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefijo del bucket de logs, el trail, el flow log, las reglas de Config y el presupuesto. | `string` | n/a | yes |
| <a name="input_vpc_id"></a> [vpc\_id](#input\_vpc\_id) | VPC cuyo tráfico registra el flow log. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bucket_name"></a> [bucket\_name](#output\_bucket\_name) | Bucket de logs de auditoría: CloudTrail, flow logs y Config, cada uno bajo su prefijo. |
| <a name="output_budget_name"></a> [budget\_name](#output\_budget\_name) | Presupuesto mensual, o null si no hay direcciones para avisar. |
| <a name="output_config_rule_names"></a> [config\_rule\_names](#output\_config\_rule\_names) | Reglas de AWS Config. |
| <a name="output_flow_log_id"></a> [flow\_log\_id](#output\_flow\_log\_id) | ID del flow log de la VPC. |
| <a name="output_trail_arn"></a> [trail\_arn](#output\_trail\_arn) | ARN del trail de CloudTrail. |
<!-- END_TF_DOCS -->
