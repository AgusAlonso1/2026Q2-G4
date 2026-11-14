# backup

AWS Backup de la base: un plan diario en un vault de la región del despliegue, con copia a otro
vault en la región de DR (`us-west-2`, la otra que habilita AWS Academy). Es la estrategia de
backup & restore: si se cae la región principal, la infraestructura se levanta en la otra con el
mismo Terraform y la base se restaura desde la última copia.

Recibe dos providers: el principal y `aws.dr`, el de la región de las copias. Usa `LabRole`, que
confía en `backup.amazonaws.com`. Los vaults llevan `force_destroy` para que Destroy pueda
borrarlos con sus copias.

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
| <a name="provider_aws.dr"></a> [aws.dr](#provider\_aws.dr) | >= 6.28 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [aws_backup_plan.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/backup_plan) | resource |
| [aws_backup_selection.rds](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/backup_selection) | resource |
| [aws_backup_vault.dr](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/backup_vault) | resource |
| [aws_backup_vault.primary](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/backup_vault) | resource |
| [aws_region.dr](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/region) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_db_instance_arn"></a> [db\_instance\_arn](#input\_db\_instance\_arn) | Instancia RDS que se respalda. | `string` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefijo de los vaults, el plan y la selección. | `string` | n/a | yes |
| <a name="input_retention_days"></a> [retention\_days](#input\_retention\_days) | Días que se conserva cada copia, en la región principal y en la de DR. | `number` | `7` | no |
| <a name="input_role_arn"></a> [role\_arn](#input\_role\_arn) | Rol con el que AWS Backup hace las copias. En AWS Academy, LabRole: confía en backup.amazonaws.com. | `string` | n/a | yes |
| <a name="input_schedule"></a> [schedule](#input\_schedule) | Cuándo corre el backup diario. Cron en UTC: las 7 son las 4 de Buenos Aires, fuera del horario de pagos y antes de la ventana del archivador. | `string` | `"cron(0 7 * * ? *)"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_dr_region"></a> [dr\_region](#output\_dr\_region) | Región de las copias de DR. |
| <a name="output_dr_vault_name"></a> [dr\_vault\_name](#output\_dr\_vault\_name) | Vault de DR, en la otra región. |
| <a name="output_plan_id"></a> [plan\_id](#output\_plan\_id) | Plan de backup: para correr uno a demanda con start-backup-job. |
| <a name="output_vault_name"></a> [vault\_name](#output\_vault\_name) | Vault principal, en la región del despliegue. |
<!-- END_TF_DOCS -->
