# service

Un servicio de ECS: task definition con red `awsvpc` (cada tarea tiene su ENI y su security
group), el servicio con su capacity provider y, si recibe tráfico, su target group. Escala
las tareas por CPU, por horario o por la profundidad de una cola SQS; las instancias las pone
el módulo `capacity`. Los logs del contenedor van a CloudWatch Logs.

**Escalado por cola** (`queue_scaling`). Dos alarmas de CloudWatch sobre la cola, cada una con
su política de pasos:

- **Subir:** con mensajes visibles (`ApproximateNumberOfMessagesVisible > 0` en el último
  minuto), una tarea más; dos si hay `backlog_step` (10) o más. Mientras la alarma sigue
  activa, Application Auto Scaling la vuelve a aplicar después del cooldown (5 minutos, lo que
  tarda en arrancar una instancia), hasta `max_count`.
- **Bajar:** cuando visibles, en curso y demorados suman 0 durante `idle_minutes` (10), deja el
  servicio en `min_count`. Contar los en curso evita cortar una tarea que está trabajando, y
  los demorados, bajar a cero entre dos intentos de un mismo mensaje.

Con `min_count = 0` la primera tarea arranca desde cero: SQS publica sus métricas una vez por
minuto, y si la cola estuvo inactiva puede tardar varios minutos más en volver a publicarlas;
después la alarma, ECS y una instancia nueva. Cuenta entre 5 y 20 minutos.

`deployment_minimum_healthy_percent = 0` y `deployment_maximum_percent = 100` reemplazan las
tareas sin sumar ninguna: sirve para un worker que devuelve su trabajo a la cola al apagarse.

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
| [aws_appautoscaling_policy.cpu](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/appautoscaling_policy) | resource |
| [aws_appautoscaling_policy.queue_in](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/appautoscaling_policy) | resource |
| [aws_appautoscaling_policy.queue_out](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/appautoscaling_policy) | resource |
| [aws_appautoscaling_scheduled_action.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/appautoscaling_scheduled_action) | resource |
| [aws_appautoscaling_target.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/appautoscaling_target) | resource |
| [aws_cloudwatch_log_group.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_log_group) | resource |
| [aws_cloudwatch_metric_alarm.queue_in](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_cloudwatch_metric_alarm.queue_out](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_ecs_service.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecs_service) | resource |
| [aws_ecs_task_definition.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecs_task_definition) | resource |
| [aws_region.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/region) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_capacity_provider_name"></a> [capacity\_provider\_name](#input\_capacity\_provider\_name) | Capacity provider donde se ubican las tareas. | `string` | n/a | yes |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Cluster de ECS, con el capacity provider del servicio ya asociado. | `string` | n/a | yes |
| <a name="input_command"></a> [command](#input\_command) | Binario que corre el contenedor dentro de la imagen. | `list(string)` | n/a | yes |
| <a name="input_cpu"></a> [cpu](#input\_cpu) | CPU reservada por tarea, en unidades de ECS (1024 = 1 vCPU). Es la base de la métrica de CPU del autoscaling. | `number` | n/a | yes |
| <a name="input_cpu_target"></a> [cpu\_target](#input\_cpu\_target) | CPU promedio, en %, que mantiene el autoscaling. null para no escalar por CPU. | `number` | `null` | no |
| <a name="input_deployment_maximum_percent"></a> [deployment\_maximum\_percent](#input\_deployment\_maximum\_percent) | Tope de tareas durante un deploy, en % de las deseadas. 100 reemplaza sin tareas de más. | `number` | `200` | no |
| <a name="input_deployment_minimum_healthy_percent"></a> [deployment\_minimum\_healthy\_percent](#input\_deployment\_minimum\_healthy\_percent) | Tareas que tienen que seguir corriendo durante un deploy, en % de las deseadas. 0 deja que ECS pare las viejas antes de arrancar las nuevas. | `number` | `50` | no |
| <a name="input_environment"></a> [environment](#input\_environment) | Variables de entorno del contenedor. Sin secretos: quedan a la vista en la task definition. | `map(string)` | `{}` | no |
| <a name="input_image"></a> [image](#input\_image) | Imagen del contenedor, con su tag. | `string` | n/a | yes |
| <a name="input_log_kms_key_arn"></a> [log\_kms\_key\_arn](#input\_log\_kms\_key\_arn) | Clave KMS con la que se cifran los logs. null: la de CloudWatch. | `string` | `null` | no |
| <a name="input_log_retention_days"></a> [log\_retention\_days](#input\_log\_retention\_days) | Días que se conservan los logs del servicio. | `number` | `14` | no |
| <a name="input_max_count"></a> [max\_count](#input\_max\_count) | Máximo de tareas. | `number` | n/a | yes |
| <a name="input_memory"></a> [memory](#input\_memory) | Memoria máxima por tarea, en MiB. | `number` | n/a | yes |
| <a name="input_min_count"></a> [min\_count](#input\_min\_count) | Mínimo de tareas. También es la cantidad con la que arranca el servicio. | `number` | n/a | yes |
| <a name="input_name"></a> [name](#input\_name) | Nombre del servicio, de su task definition y de su log group. | `string` | n/a | yes |
| <a name="input_port"></a> [port](#input\_port) | Puerto del contenedor. null si el servicio no recibe tráfico. | `number` | `null` | no |
| <a name="input_queue_scaling"></a> [queue\_scaling](#input\_queue\_scaling) | Escalado por la profundidad de una cola SQS: suma una tarea si hay mensajes visibles (dos desde backlog\_step) y baja a min\_count cuando la cola lleva idle\_minutes sin mensajes visibles, en curso ni demorados. null para no escalar por cola. | <pre>object({<br/>    queue_name   = string<br/>    backlog_step = optional(number, 10)<br/>    idle_minutes = optional(number, 10)<br/>  })</pre> | `null` | no |
| <a name="input_role_arn"></a> [role\_arn](#input\_role\_arn) | Rol de ejecución y de la tarea. En AWS Academy no se pueden crear roles: es el del laboratorio. | `string` | n/a | yes |
| <a name="input_schedule"></a> [schedule](#input\_schedule) | Acciones programadas: fijan la cantidad de tareas a una hora. Cron de Application Auto Scaling. | <pre>map(object({<br/>    schedule = string<br/>    tasks    = number<br/>  }))</pre> | `{}` | no |
| <a name="input_schedule_time_zone"></a> [schedule\_time\_zone](#input\_schedule\_time\_zone) | Zona horaria de las acciones programadas. | `string` | `"America/Argentina/Buenos_Aires"` | no |
| <a name="input_secrets"></a> [secrets](#input\_secrets) | Variables secretas: nombre → ARN de Secrets Manager (arn:clave-json::version-id). ECS lee el valor al arrancar la tarea, con el rol de ejecución. | `map(string)` | `{}` | no |
| <a name="input_security_group_id"></a> [security\_group\_id](#input\_security\_group\_id) | SG de la ENI de cada tarea. | `string` | n/a | yes |
| <a name="input_subnet_ids"></a> [subnet\_ids](#input\_subnet\_ids) | Subredes de las tareas (red awsvpc: cada tarea tiene su ENI). Las mismas que las de sus instancias. | `list(string)` | n/a | yes |
| <a name="input_target_group_arn"></a> [target\_group\_arn](#input\_target\_group\_arn) | Target group donde ECS registra las tareas. null si el servicio no va detrás del balanceador. | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_log_group_name"></a> [log\_group\_name](#output\_log\_group\_name) | Log group del servicio: ahí escribe el contenedor. |
| <a name="output_name"></a> [name](#output\_name) | Nombre del servicio de ECS. |
<!-- END_TF_DOCS -->
