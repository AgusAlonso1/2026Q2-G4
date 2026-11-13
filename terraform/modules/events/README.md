# events

Eventos del ledger. La API publica cada orden terminada en el topic `events`, que los reparte:

- **Webhook:** SNS → SQS (+DLQ) → Lambda, que firma el aviso con HMAC y hace el POST. Los
  reintentos son de SQS; lo que los agota queda en la DLQ y dispara una alarma.
- **Mails:** SNS → Lambda → SES, sin cola, filtrado a los eventos que le importan al titular.
  Los reintentos son de Lambda; lo que falla va al topic de alertas.
- **Alertas:** topic aparte, con suscripciones de email para operación.

## Contrato del evento

La API publica en `SNS_TOPIC_ARN` un mensaje por orden que llega a un estado terminal.

- **Atributos del mensaje** (`String`): `tipo` y `estado`. Con ellos filtra la suscripción del
  mailer.
- **Cuerpo** (JSON):

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
  "referencia": "alquiler",
  "titulares": { "origen": "<sub de Cognito>", "destino": "<sub de Cognito>" }
}
```

`evento_id` es `<orden_id>:<estado>`: el mismo evento tiene siempre el mismo id, así que el
receptor del webhook puede descartar los duplicados. `titulares` lo usa sólo el mailer: la
Lambda del webhook lo saca antes de firmar y mandar el aviso.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
