# monitoring

Observabilidad del lado síncrono: lo que el módulo `events` no cubre. Todo va a CloudWatch y las
alarmas avisan al topic de alertas de operación (`ledger-alerts`), el mismo de la alarma de la
DLQ, al dispararse y al volver a OK.

- **Métricas desde el log de la API.** Cada request deja una línea JSON con `msg = "request"`,
  `ruta`, `estado` y `ms`. Tres *metric filters* la convierten en `Pedidos`, `Latencia` y
  `Respuestas5xx`, en el namespace `<prefijo>/api`, sin tocar el código. `/health` no cuenta: el
  NLB lo pide cada pocos segundos.
- **Alarmas.** 5xx de la API, latencia p99, targets sanos del NLB por debajo del mínimo de tareas,
  y CPU, conexiones, memoria libre y disco libre de RDS.
- **Dashboard.** Tareas, CPU e instancias de ECS; targets del NLB; latencia, pedidos y 5xx de la
  API; RDS; la cola y la Lambda del webhook; y los pedidos que bloquea el WAF del login. Arriba,
  el estado de todas las alarmas.
- **Consultas guardadas de Logs Insights.** Los pedidos más lentos, la latencia por ruta, los
  5xx, los errores de los binarios y los avisos del webhook que no se entregaron.

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
| [aws_cloudwatch_dashboard.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_dashboard) | resource |
| [aws_cloudwatch_log_metric_filter.api](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_log_metric_filter) | resource |
| [aws_cloudwatch_metric_alarm.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_cloudwatch_query_definition.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_query_definition) | resource |
| [aws_caller_identity.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/caller_identity) | data source |
| [aws_region.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/region) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_alarm_topic_arn"></a> [alarm\_topic\_arn](#input\_alarm\_topic\_arn) | Topic SNS de alertas de operación: las alarmas publican ahí cuando se disparan y cuando vuelven a OK. | `string` | n/a | yes |
| <a name="input_api_cpu_target"></a> [api\_cpu\_target](#input\_api\_cpu\_target) | CPU objetivo del autoscaling de la API, en %. Se dibuja como referencia en el dashboard. | `number` | n/a | yes |
| <a name="input_api_log_group_name"></a> [api\_log\_group\_name](#input\_api\_log\_group\_name) | Log group de la API. Cada request deja una línea JSON con msg = "request", ruta, estado y ms. | `string` | n/a | yes |
| <a name="input_api_min_tasks"></a> [api\_min\_tasks](#input\_api\_min\_tasks) | Mínimo de tareas de la API. Con menos targets sanos en el NLB, la alarma se dispara. | `number` | n/a | yes |
| <a name="input_api_service_name"></a> [api\_service\_name](#input\_api\_service\_name) | Servicio de ECS de la API. | `string` | n/a | yes |
| <a name="input_archive_log_group_name"></a> [archive\_log\_group\_name](#input\_archive\_log\_group\_name) | Log group del archivador, para su consulta guardada. | `string` | n/a | yes |
| <a name="input_archive_service_name"></a> [archive\_service\_name](#input\_archive\_service\_name) | Servicio de ECS del archivador. | `string` | n/a | yes |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Cluster de ECS. Container Insights publica sus métricas con esta dimensión. | `string` | n/a | yes |
| <a name="input_db_connections_threshold"></a> [db\_connections\_threshold](#input\_db\_connections\_threshold) | Conexiones abiertas a RDS. El diseño reparte db\_max\_connections (60) y una db.t3.micro acepta unas 80: pasar este valor quiere decir que algo se salió del cálculo. | `number` | `70` | no |
| <a name="input_db_cpu_threshold"></a> [db\_cpu\_threshold](#input\_db\_cpu\_threshold) | CPU de RDS, en %, sostenida 10 minutos. | `number` | `80` | no |
| <a name="input_db_free_memory_threshold_mb"></a> [db\_free\_memory\_threshold\_mb](#input\_db\_free\_memory\_threshold\_mb) | Memoria libre de RDS, en MB, por debajo de la cual se dispara la alarma (10 minutos). | `number` | `100` | no |
| <a name="input_db_free_storage_threshold_gb"></a> [db\_free\_storage\_threshold\_gb](#input\_db\_free\_storage\_threshold\_gb) | Espacio libre en el disco de RDS, en GB. El disco es fijo, sin autoescalado de almacenamiento. | `number` | `2` | no |
| <a name="input_db_instance_identifier"></a> [db\_instance\_identifier](#input\_db\_instance\_identifier) | Identificador de la instancia RDS. | `string` | n/a | yes |
| <a name="input_errors_5xx_threshold"></a> [errors\_5xx\_threshold](#input\_errors\_5xx\_threshold) | Respuestas 5xx de la API en 5 minutos a partir de las cuales se dispara la alarma. | `number` | `5` | no |
| <a name="input_latency_p99_threshold_ms"></a> [latency\_p99\_threshold\_ms](#input\_latency\_p99\_threshold\_ms) | Latencia p99 de la API, en ms, por encima de la cual se dispara la alarma (3 minutos seguidos). No cuenta /health. | `number` | `1000` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefijo de las alarmas, el dashboard, las métricas y las consultas guardadas. | `string` | n/a | yes |
| <a name="input_nlb_arn_suffix"></a> [nlb\_arn\_suffix](#input\_nlb\_arn\_suffix) | Sufijo del ARN del NLB (net/<nombre>/<id>): la dimensión LoadBalancer de sus métricas. | `string` | n/a | yes |
| <a name="input_target_group_arn_suffix"></a> [target\_group\_arn\_suffix](#input\_target\_group\_arn\_suffix) | Sufijo del ARN del target group de la API (targetgroup/<nombre>/<id>): la dimensión TargetGroup. | `string` | n/a | yes |
| <a name="input_waf_web_acl_name"></a> [waf\_web\_acl\_name](#input\_waf\_web\_acl\_name) | Web ACL del WAF del login, para el panel de pedidos bloqueados. | `string` | n/a | yes |
| <a name="input_webhook_dlq_name"></a> [webhook\_dlq\_name](#input\_webhook\_dlq\_name) | DLQ del webhook. | `string` | n/a | yes |
| <a name="input_webhook_function_name"></a> [webhook\_function\_name](#input\_webhook\_function\_name) | Lambda del webhook. | `string` | n/a | yes |
| <a name="input_webhook_log_group_name"></a> [webhook\_log\_group\_name](#input\_webhook\_log\_group\_name) | Log group de la Lambda del webhook, para su consulta guardada. | `string` | n/a | yes |
| <a name="input_webhook_queue_name"></a> [webhook\_queue\_name](#input\_webhook\_queue\_name) | Cola SQS del webhook. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_alarm_names"></a> [alarm\_names](#output\_alarm\_names) | Alarmas que crea el módulo. Publican en el topic de alertas al dispararse y al volver a OK. |
| <a name="output_dashboard_url"></a> [dashboard\_url](#output\_dashboard\_url) | Dashboard de CloudWatch con el cómputo, la entrada, la base y los eventos. |
| <a name="output_metric_namespace"></a> [metric\_namespace](#output\_metric\_namespace) | Namespace de las métricas que salen del log de la API (Pedidos, Latencia, Respuestas5xx). |
| <a name="output_query_names"></a> [query\_names](#output\_query\_names) | Consultas guardadas de Logs Insights. |
<!-- END_TF_DOCS -->
