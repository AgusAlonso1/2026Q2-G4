# events

Eventos del ledger. La API publica cada orden terminada en el topic `events`, que los reparte:

- **Webhook:** SNS → SQS (+DLQ) → Lambda, que firma el aviso con HMAC y hace el POST. Los
  reintentos son de SQS; lo que los agota queda en la DLQ y dispara una alarma.
- **Alertas:** topic aparte, con suscripciones de email para operación.
- **Titulares:** topic `titulares`, con una suscripción de email por titular. Lo usa la API para
  avisarle a cada uno lo suyo (hoy, que su exportación está lista o que no se pudo armar).

Además, el módulo tiene la cola de las exportaciones, que no cuelga de `events`:

- **Exportaciones:** SQS `exports` (+DLQ). La API manda el id de cada exportación pedida y la
  consume el exportador, que escala con esta cola. El exportador reintenta con espera creciente
  hasta `exports_max_attempts`, y la exportación que los agota la manda a la DLQ. La cola, por
  su cuenta, pasa a la DLQ lo que se recibió `exports_max_attempts + 5` veces sin resolverse.
  Dos alarmas cubren la DLQ: `exports-dlq-sent` avisa por cada exportación que manda el
  exportador, aunque la DLQ ya tenga otras, y `exports-dlq`, como la del webhook, mientras la
  DLQ tenga mensajes, también los que pasó la cola. Otra alarma avisa si el mensaje más viejo
  lleva más de `exports_max_age_seconds` (una hora) en la cola: el exportador no lo tomó o
  lleva todo ese tiempo con él.

## Contrato del evento

La API publica en `SNS_TOPIC_ARN` un mensaje por orden que llega a un estado terminal.

- **Atributos del mensaje** (`String`): `tipo` y `estado`, para filtrar suscripciones sin leer
  el cuerpo.
- **Cuerpo** (JSON), que la Lambda firma y entrega tal cual:

```json
{
  "evento_id": "42:liquidada",
  "orden_id": 42,
  "tipo": "transferencia",
  "estado": "liquidada",
  "motivo": "",
  "origen_id": 7,
  "destino_id": 9,
  "monto_centavos": 150050,
  "referencia": "alquiler"
}
```

`evento_id` es `<orden_id>:<estado>`: el mismo evento tiene siempre el mismo id, así que el
receptor del webhook puede descartar los duplicados.

## Contrato de los avisos a titulares

La API suscribe el email de cada titular a `TITULARES_TOPIC_ARN` con el *filter policy*
`{"titular": ["<sub de Cognito>"]}`, y publica cada aviso con el atributo `titular` (`String`)
igual a ese `sub`: SNS se lo entrega sólo a esa suscripción. El cuerpo es texto plano, porque
llega como mail. SNS admite 200 *filter policies* por topic.

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.9 |
| <a name="requirement_archive"></a> [archive](#requirement\_archive) | >= 2.4 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.28 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_archive"></a> [archive](#provider\_archive) | >= 2.4 |
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.28 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [aws_cloudwatch_log_group.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_log_group) | resource |
| [aws_cloudwatch_metric_alarm.exports_age](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_cloudwatch_metric_alarm.exports_dlq](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_cloudwatch_metric_alarm.exports_dlq_sent](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_cloudwatch_metric_alarm.webhook_dlq](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_lambda_event_source_mapping.webhook](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lambda_event_source_mapping) | resource |
| [aws_lambda_function.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lambda_function) | resource |
| [aws_secretsmanager_secret.webhook](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/secretsmanager_secret) | resource |
| [aws_secretsmanager_secret_version.webhook](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/secretsmanager_secret_version) | resource |
| [aws_sns_topic.alerts](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic) | resource |
| [aws_sns_topic.events](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic) | resource |
| [aws_sns_topic.titulares](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic) | resource |
| [aws_sns_topic_subscription.alerts](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic_subscription) | resource |
| [aws_sns_topic_subscription.webhook](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic_subscription) | resource |
| [aws_sqs_queue.exports](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue) | resource |
| [aws_sqs_queue.exports_dlq](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue) | resource |
| [aws_sqs_queue.webhook](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue) | resource |
| [aws_sqs_queue.webhook_dlq](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue) | resource |
| [aws_sqs_queue_policy.webhook](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sqs_queue_policy) | resource |
| [archive_file.this](https://registry.terraform.io/providers/hashicorp/archive/latest/docs/data-sources/file) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_exports_max_age_seconds"></a> [exports\_max\_age\_seconds](#input\_exports\_max\_age\_seconds) | Antigüedad, en segundos, del mensaje más viejo de la cola de exportaciones a partir de la cual se avisa a operación: con el exportador en marcha, nada espera tanto. | `number` | `3600` | no |
| <a name="input_exports_max_attempts"></a> [exports\_max\_attempts](#input\_exports\_max\_attempts) | Cuántas veces se intenta armar una exportación antes de marcarla fallida; el exportador manda las fallidas a la DLQ. La cola pasa a la DLQ por su cuenta lo que se recibe cinco veces más que esto sin resolverse (por ejemplo, una tarea que muere una y otra vez con el mismo mensaje). Tiene que coincidir con el que usan la API y el exportador. | `number` | n/a | yes |
| <a name="input_exports_visibility_timeout_seconds"></a> [exports\_visibility\_timeout\_seconds](#input\_exports\_visibility\_timeout\_seconds) | Segundos que un mensaje de la cola de exportaciones queda oculto mientras una tarea lo atiende. La tarea la extiende mientras trabaja, así que acota cuánto tarda en reaparecer si la tarea muere. Tiene que coincidir con el que usan la API y el exportador. | `number` | n/a | yes |
| <a name="input_kms_key_arn"></a> [kms\_key\_arn](#input\_kms\_key\_arn) | Clave KMS de los topics, las colas, los logs y el secreto. Su política tiene que habilitar a sns.amazonaws.com y cloudwatch.amazonaws.com. | `string` | n/a | yes |
| <a name="input_lambda_role_arn"></a> [lambda\_role\_arn](#input\_lambda\_role\_arn) | Rol de ejecución de la Lambda. En AWS Academy, el LabRole preexistente. | `string` | n/a | yes |
| <a name="input_lambda_runtime"></a> [lambda\_runtime](#input\_lambda\_runtime) | Runtime de la Lambda. Node: el SDK de AWS viene en el runtime, sin dependencias ni paso de build. | `string` | `"nodejs22.x"` | no |
| <a name="input_lambda_timeout"></a> [lambda\_timeout](#input\_lambda\_timeout) | Timeout de la Lambda, en segundos. La visibilidad de la cola del webhook es 6 veces este valor. | `number` | `10` | no |
| <a name="input_log_retention_days"></a> [log\_retention\_days](#input\_log\_retention\_days) | Días que se conservan los logs de la Lambda. | `number` | `7` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefijo de los nombres: topics, colas, Lambda y secreto. | `string` | n/a | yes |
| <a name="input_notification_emails"></a> [notification\_emails](#input\_notification\_emails) | Emails que se suscriben al topic de alertas de operación. | `list(string)` | `[]` | no |
| <a name="input_webhook_max_attempts"></a> [webhook\_max\_attempts](#input\_webhook\_max\_attempts) | Cuántas veces se intenta entregar un aviso antes de mandarlo a la DLQ. | `number` | `5` | no |
| <a name="input_webhook_secret"></a> [webhook\_secret](#input\_webhook\_secret) | Secreto HMAC de los avisos. Tiene que coincidir con el del que recibe. | `string` | n/a | yes |
| <a name="input_webhook_source_dir"></a> [webhook\_source\_dir](#input\_webhook\_source\_dir) | Carpeta con el handler.mjs de la Lambda del webhook. | `string` | n/a | yes |
| <a name="input_webhook_url"></a> [webhook\_url](#input\_webhook\_url) | Adonde se entregan los avisos. Vacía: la Lambda descarta los mensajes. | `string` | `""` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_alerts_topic_arn"></a> [alerts\_topic\_arn](#output\_alerts\_topic\_arn) | Topic de alertas de operación: las alarmas de las DLQ del webhook y de las exportaciones, y la de antigüedad de la cola de exportaciones. |
| <a name="output_exports_dlq_url"></a> [exports\_dlq\_url](#output\_exports\_dlq\_url) | DLQ de las exportaciones (EXPORTACIONES\_DLQ\_URL): las que agotaron los reintentos y los mensajes que la cola no pudo entregar. |
| <a name="output_exports_queue_name"></a> [exports\_queue\_name](#output\_exports\_queue\_name) | Nombre de la cola de exportaciones: la dimensión de las métricas con las que escala el exportador. |
| <a name="output_exports_queue_url"></a> [exports\_queue\_url](#output\_exports\_queue\_url) | Cola de las exportaciones pendientes (EXPORTACIONES\_QUEUE\_URL): la API encola cada pedido y la consume el exportador. |
| <a name="output_titulares_topic_arn"></a> [titulares\_topic\_arn](#output\_titulares\_topic\_arn) | Topic de los avisos por mail a cada titular (TITULARES\_TOPIC\_ARN): cada suscripción filtra por su titular. |
| <a name="output_topic_arn"></a> [topic\_arn](#output\_topic\_arn) | Topic donde la API publica cada orden terminada (SNS\_TOPIC\_ARN). |
| <a name="output_webhook_dlq_url"></a> [webhook\_dlq\_url](#output\_webhook\_dlq\_url) | DLQ del webhook: los avisos que agotaron los reintentos. |
| <a name="output_webhook_queue_url"></a> [webhook\_queue\_url](#output\_webhook\_queue\_url) | Cola de entrega del webhook. |
<!-- END_TF_DOCS -->
