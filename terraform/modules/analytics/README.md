# analytics

Consultas SQL con Athena sobre el archivo del ledger, sin tocar la base transaccional. El
archivador deja cada mes en `transaction-history/periodo=AAAA-MM/`, en uno o más archivos Parquet
(`parte-000.parquet`, `parte-001.parquet`, …), cada uno con un rango de cuentas y ordenado por
cuenta. El módulo lo cataloga en Glue como la tabla `asientos`, particionada por `periodo`.

- **Parquet.** Athena lee sólo las columnas que usa la consulta, reparte los archivos y sus
  bloques entre varios workers y, con `WHERE cuenta_id = …`, saltea los bloques que no contienen
  la cuenta (cada bloque guarda el mínimo y el máximo de cada columna). Escanea mucho menos que
  sobre JSON, y Athena cobra por byte escaneado.
- **Particiones por proyección.** La tabla no guarda particiones en el catálogo: Athena las
  calcula desde `history_start` hasta el mes actual, a partir de las carpetas `periodo=AAAA-MM/`.
  Un mes recién archivado se puede consultar enseguida, sin `MSCK REPAIR TABLE` ni crawler: el
  esquema y las carpetas los define el propio archivador, así que no hay nada que descubrir.
- **Workgroup con la configuración forzada.** Los resultados van a `athena-results/` del mismo
  bucket, cifrados con la clave del proyecto, y cada consulta tiene un tope de bytes escaneados.
  `storage` los expira a los 7 días; como el bucket está versionado, pasan a ser una versión
  anterior y se borran 30 días después.
- **Consultas guardadas** en el workgroup: `partida-doble`, `volumen-mensual`,
  `cuentas-mas-activas` y `movimientos-de-cuenta` (con `cuenta_id` como parámetro).

`creado_en` es un `timestamp` en UTC. `periodo` no es una columna del archivo: es la partición, en
formato `AAAA-MM`.

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.9 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.37 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.37 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [aws_athena_named_query.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/athena_named_query) | resource |
| [aws_athena_workgroup.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/athena_workgroup) | resource |
| [aws_glue_catalog_database.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/glue_catalog_database) | resource |
| [aws_glue_catalog_table.asientos](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/glue_catalog_table) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_bucket_name"></a> [bucket\_name](#input\_bucket\_name) | Bucket del proyecto: de ahí lee el archivo y ahí escribe Athena los resultados. | `string` | n/a | yes |
| <a name="input_bytes_scanned_cutoff_per_query"></a> [bytes\_scanned\_cutoff\_per\_query](#input\_bytes\_scanned\_cutoff\_per\_query) | Tope de bytes que puede escanear una consulta del workgroup: la cancela si lo pasa. Athena cobra por byte escaneado. | `number` | `1073741824` | no |
| <a name="input_history_prefix"></a> [history\_prefix](#input\_history\_prefix) | Prefijo del archivo del ledger en el bucket. Debajo, un prefijo periodo=AAAA-MM por mes archivado. | `string` | `"transaction-history/"` | no |
| <a name="input_history_start"></a> [history\_start](#input\_history\_start) | Primer mes (AAAA-MM) que proyecta la tabla. Los meses anteriores no se ven en Athena. | `string` | `"2024-01"` | no |
| <a name="input_kms_key_arn"></a> [kms\_key\_arn](#input\_kms\_key\_arn) | Clave KMS con la que se cifran los resultados de las consultas (SSE-KMS). | `string` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefijo de los nombres: workgroup de Athena y base del catálogo de Glue (con los guiones pasados a guion bajo). | `string` | n/a | yes |
| <a name="input_results_prefix"></a> [results\_prefix](#input\_results\_prefix) | Prefijo del bucket donde Athena escribe los resultados de las consultas. | `string` | `"athena-results/"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_database_name"></a> [database\_name](#output\_database\_name) | Base del catálogo de Glue. |
| <a name="output_named_query_ids"></a> [named\_query\_ids](#output\_named\_query\_ids) | Ids de las consultas guardadas, por nombre. |
| <a name="output_results_location"></a> [results\_location](#output\_results\_location) | Dónde escribe Athena los resultados de las consultas. |
| <a name="output_table_name"></a> [table\_name](#output\_table\_name) | Tabla de los asientos archivados. |
| <a name="output_workgroup_name"></a> [workgroup\_name](#output\_workgroup\_name) | Workgroup de Athena: hay que elegirlo en el editor para usar su configuración y sus consultas guardadas. |
<!-- END_TF_DOCS -->
