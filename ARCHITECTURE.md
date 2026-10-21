# Arquitectura — Núcleo Transaccional Elástico

Qué despliega este repositorio y por qué. El código Terraform es la fuente de verdad; cómo
desplegarlo está en el [README](README.md).

## 1. Contexto

Núcleo transaccional de un PSP: cada usuario tiene una cuenta virtual con la que transfiere a
otras cuentas, ingresa dinero desde su banco y lo retira hacia afuera a través de un proveedor.
Cada operación se liquida dentro del mismo request, de forma idempotente, contra un ledger de
partida doble. Lo único diferido es el archivado de los meses viejos a S3.

Restricciones del diseño:

- **AWS Academy**: no se pueden crear usuarios ni roles de IAM (se usan `LabRole` y
  `LabInstanceProfile`) y sólo está habilitada `us-east-1`.
- **Reproducible**: todo se despliega y se destruye desde GitHub Actions en cualquier cuenta,
  sin editar nada. Los nombres únicos llevan el ID de la cuenta.
- **Costo acotado**: un solo NAT, la API en Graviton y el archivador apagado fuera de su ventana.

## 2. Red

VPC `10.0.0.0/16` con el módulo `terraform-aws-modules/vpc/aws`, dos AZ y cuatro capas, cada
una con su tabla de ruteo:

| Capa | CIDR (AZ a, AZ b) | Salida | Qué corre |
|---|---|---|---|
| `public` | `10.0.0.0/24`, `10.0.1.0/24` | IGW | NLB, NAT |
| `app` | `10.0.10.0/24`, `10.0.11.0/24` | NAT y gateway endpoint de S3 | API, interface endpoints |
| `archive` | `10.0.12.0/24`, `10.0.13.0/24` | Sólo el gateway endpoint de S3 | Archivador |
| `data` | `10.0.20.0/24`, `10.0.21.0/24` | Ninguna | RDS (primaria y standby) |

Los security groups se referencian entre sí, nunca por rango de direcciones:

| SG | Entra | Sale |
|---|---|---|
| `nlb` | 80 desde Internet | La API |
| `app` | Puerto de la app desde `nlb` | `data` (5432), S3, Internet por el NAT (proveedor y webhook) |
| `archive` | Nada | `data` (5432), S3 |
| `data` | 5432 desde `app` y `archive` | Nada |
| `host` (instancias de ECS) | Nada | `endpoints` (443), S3 |
| `endpoints` | 443 desde `host` | Nada |

No hay SSH. Las NACL quedan en su regla por defecto: el aislamiento ya lo dan el ruteo (`data` y
`archive` no tienen ruta a Internet) y los security groups. Una NACL no tiene estado, así que
repetir esas reglas obligaría a abrir a mano los puertos efímeros de vuelta sin ganar nada.

### Entrada: por qué un NLB

Fue la corrección de la segunda entrega: por la naturaleza de la aplicación, el balanceador
correcto es uno de red. Un PSP es un núcleo transaccional, no un sitio web:

- **Latencia en cada pago.** Hay una persona esperando en la caja. El NLB reenvía la conexión
  TCP sin abrir el HTTP; un ALB termina la conexión y agrega latencia por request.
- **Picos bruscos.** El volumen se multiplica en el cobro de haberes. El NLB escala sin
  precalentamiento; un ALB crece de a poco y ante un salto brusco puede responder 503.
- **Conexiones largas** de comercios, terminales y proveedores, que el NLB sostiene sin
  cortarlas por request.
- **Protocolos que no son HTTP.** La red de pagos (adquirentes, terminales POS) habla ISO 8583
  sobre TCP; un ALB sólo entiende HTTP.
- **TLS de punta a punta.** Con un NLB el TLS puede terminar en la aplicación, sin que un
  intermediario descifre los datos del pago (PCI DSS). Hoy el listener es TCP:80 porque el
  laboratorio no tiene dominio para un certificado.
- **IP fija por zona**, que bancos y proveedores piden para sus listas blancas; las de un ALB
  cambian.
- **No hace falta nada de lo que agrega un ALB**: detrás hay un solo servicio, sin ruteo por
  path ni por host, y la autenticación la hace la aplicación.

El NLB está en las dos AZ con balanceo entre zonas y reenvía a la IP de cada tarea (targets `ip`,
porque con red `awsvpc` cada tarea tiene la suya). Tiene su propio security group, así que `app`
acepta "desde el NLB" y no desde un rango. El health check es HTTP a `/health`, que responde 200
mientras el proceso esté vivo e informa en el cuerpo si llega a la base: una caída de la base no
saca todas las tareas del NLB a la vez.

Lo que se resigna: la métrica de requests por target es sólo del ALB, así que la API escala por
CPU, y no se puede poner WAF delante (va en ALB, CloudFront o API Gateway).

## 3. Cómputo

La API y el archivador corren en **ECS sobre EC2**, desde la misma imagen (cada servicio arranca
`/api` o `/archivador`), construida para amd64 y arm64. Cada uno tiene su ASG y su capacity
provider:

| | API | Archivador |
|---|---|---|
| Subredes | `app` | `archive`, sin salida a Internet |
| Tareas | 2 a 5, escalan por CPU (60 %) | 1 entre las 19:25 y las 21:30, 0 el resto |
| Instancias | `c7g.large` (Graviton), dos tareas cada una | `t3.micro`, una tarea |
| Tráfico | NLB → IP de cada tarea | Ninguno: sólo sale a la base y a S3 |

Hay dos niveles de escalado: el servicio decide cuántas tareas quiere y el capacity provider
prende o apaga instancias para alojarlas. Con `awsvpc` cada tarea ocupa una ENI de la instancia:
una `c7g.large` (3 ENIs) aloja dos tareas y una `t3.micro` (2 ENIs), una. El capacity provider
de la API apunta a tener ocupado el 80 % de las instancias, para dejar lugar de reserva: la
primera tarea nueva suele arrancar sin esperar, aunque si llegan más de las que entran tienen que
esperar a la instancia nueva. El del archivador queda en 100 % y baja a cero fuera de la ventana.

**Por qué EC2 y no Fargate.** La API es una carga base constante: siempre hay dos tareas. Para
eso EC2 es más barato por vCPU, deja elegir el hardware y permite que una tarea use en un pico la
CPU libre del host. La contra es el escalado en dos niveles, más lento, que compensa el lugar de
reserva, y que los parches de las instancias son nuestros: el launch template toma la AMI
recomendada en cada apply, pero sólo la usan las instancias nuevas.

**Por qué esos tipos.** El laboratorio no deja instancias más grandes que `large`. La API va en
`c7g.large`: no es burstable, Graviton es más barato que x86 a igual tamaño, y sus 2 vCPU
coinciden con las dos tareas de 1 vCPU que le permiten sus ENIs. El archivador es un batch de
unas horas que nadie espera: alcanza una `t3.micro`. En una cuenta propia iría en Spot, que el
laboratorio no habilita: cada período archivado se puede retomar, así que una interrupción sólo
lo atrasa.

**Instancias y endpoints.** El `user_data` configura el agente de ECS con
`ECS_IMAGE_PULL_BEHAVIOR=prefer-cached` (reutiliza la imagen del host; es seguro porque los tags
son inmutables) y `ECS_AWSVPC_BLOCK_IMDS=true` (las tareas no pueden leer las credenciales de la
instancia). El agente habla con ECS, ECR, CloudWatch Logs y Secrets Manager por **interface
endpoints**, porque las subredes de `archive` no tienen salida; las capas de la imagen vienen de
S3 por el gateway endpoint. Los endpoints están en las subredes de `app` y sólo aceptan a los
hosts de ECS y credenciales de la propia cuenta. El gateway endpoint de S3 tiene una política que
sólo deja llegar al bucket del proyecto y al de las capas de ECR: el archivador, que no tiene otra
salida, no puede copiar datos a un bucket de otra cuenta.

**Deploys inmutables.** Nada se modifica en caliente. Una versión nueva del backend es una imagen
nueva en ECR (tags inmutables) y una revisión nueva de la task definition, y ECS reemplaza las
tareas de a tandas, sin bajar de la mitad ni pasar del doble. Una tarea recibe tráfico recién
cuando pasa el health check, y si las nuevas no arrancan el circuit breaker vuelve a la revisión
anterior (salvo que la nueva ya haya aplicado una migración: ahí la salida es otro deploy hacia
adelante). No se usa `prevent_destroy` porque Destroy tiene que poder borrar todo, ni hace falta
`create_before_destroy`, porque los reemplazos los hace ECS.

La API llega a 5 tareas porque su ASG admite una instancia más que el máximo, y el laboratorio no
deja un ASG de más de 6. El cluster tiene **Container Insights** (CPU, memoria y red por servicio
y por tarea) y ECR escanea cada imagen al subirla (`scan_on_push`, el escaneo básico, sin costo).

## 4. Datos

RDS PostgreSQL 18 Multi-AZ (`db.t3.micro`) en `data`, sin acceso público, cifrada con la clave
del proyecto y con 7 días de backups. La standby está en la otra AZ con réplica sincrónica y
failover sin cambiar el endpoint; no atiende tráfico.

**Por qué relacional y no DynamoDB.** Una transferencia son dos asientos y dos saldos que se
escriben juntos o no se escribe ninguno, con integridad referencial. Validar fondos necesita un
bloqueo de fila: la liquidación hace `SELECT … FOR UPDATE` sobre `saldos`, en orden de id de
cuenta para que dos transferencias cruzadas no se traben. La idempotencia es una restricción
`UNIQUE (tipo, idempotency_key)`: un reintento devuelve la orden que ya existe. Y los extractos
son agregaciones por rango de fechas. En DynamoDB todo eso habría que construirlo en la
aplicación.

**Modelo del saldo.** `asientos` es la fuente de verdad, de sólo anexado y particionada por mes;
`saldos` es el saldo vigente, que se actualiza en la misma transacción y es lo que se bloquea, así
que validar fondos cuesta lo mismo con cualquier cantidad de asientos; `saldos_apertura` guarda el
saldo al corte de cada archivado, para seguir derivándolo sin los meses viejos.

**Archivado.** El archivador toma un período (`FOR UPDATE SKIP LOCKED`), hace `DETACH PARTITION`,
lo sube a `transaction-history/periodo=AAAA-MM/asientos.ndjson.gz` (prefijo estilo Hive,
consultable con Athena) y borra la partición, sin un `DELETE` masivo que compita con la
liquidación. El extracto de un mes archivado lo arma la API desde S3.

**Conexiones.** Cada tarea abre como máximo `pool_max_conns`, que va en la `DB_URL`: se reparte
`db_max_connections` (60, debajo de las ~80 de una `db.t3.micro`) como si los dos servicios
estuvieran a la vez en el doble de su máximo de tareas, que es el pico de un deploy.

## 5. Secretos y cifrado

Las claves de los binarios (la `DB_URL` con la password, admin, mock, proveedor y webhook) no
van en la task definition, donde se verían desde la consola. Terraform las genera
(`random_password`) y las guarda en **Secrets Manager**, un secreto por servicio con sólo lo que
usa: `ledger/api` las cinco y `ledger/archive` la `DB_URL`. La task definition guarda una
referencia (`valueFrom` con `<arn>:<clave>::<version-id>`) y ECS lee el valor al arrancar la
tarea. El `version-id` hace que un valor nuevo genere una revisión y ECS reemplace las tareas.
El client secret de Cognito está en `ledger/callback`, y la Lambda sólo recibe su ARN.

El contenedor recibe los valores como variables de entorno al arrancar, porque es lo que lee el
binario, pero no están en el código, en el repositorio ni en la configuración visible. No hay
rotación automática: necesita una Lambda de rotación con su rol, que el laboratorio no deja
crear; rotar es cambiar el valor en Terraform. Los secretos se crean con
`recovery_window_in_days = 0` para que el próximo Deploy no choque con el nombre del anterior.

**Clave KMS propia.** Con las claves que administra AWS todo se cifra igual, pero no se pueden
gestionar: su política no se modifica, no se deshabilitan y la rotación no la decide uno. Un PSP
necesita poder hacerlo (PCI DSS, requisitos 3.6 y 3.7: custodios mínimos, rotación y retiro de
una clave comprometida). Por eso hay una clave propia, `alias/ledger`, con rotación anual, que
cifra los secretos, el bucket del archivo (SSE-KMS con *bucket key*), RDS, ECR y los logs. Su
política nombra a la cuenta, al `LabRole` (sólo para cifrar y descifrar) y a CloudWatch Logs.
Quedan con claves de AWS los discos de las instancias (`aws/ebs`: con una propia, el rol de Auto
Scaling tendría que estar en la política) y el bucket del sitio (AES256: lo leen usuarios
anónimos, que no pueden tener permiso sobre una clave).

## 6. Login con Cognito

Cognito sólo acepta callbacks HTTPS y el sitio se sirve por HTTP desde S3, así que el callback
pasa por API Gateway y una Lambda:

1. El frontend redirige a `login_url` (Hosted UI) agregando un `state` aleatorio.
2. Cognito vuelve a `https://<api-gateway>/callback?code=…&state=…`.
3. La Lambda canjea el `code` por tokens con el client secret y redirige al frontend con los
   tokens en el fragment (`#access_token=…`).
4. El frontend valida el `state` y llama a la API con `Authorization: Bearer <access_token>`.
5. La API valida la firma contra el JWKS del user pool, el `client_id` y `token_use=access`. Por
   CORS sólo acepta el origen del sitio.
6. La primera vez, el frontend crea la cuenta con `POST /cuentas`: queda ligada al `sub` del
   usuario, que sólo puede operar la suya.

Cerrar sesión redirige a `logout_url`, que vuelve por `/logout` de API Gateway porque Cognito
tampoco acepta logout URLs HTTP. `config.json` (`api_url`, `login_url`, `logout_url`) lo genera
Terraform.

## 7. Frontend

Sitio estático (HTML, CSS y módulos de JavaScript, sin dependencias ni build) que Terraform sube
archivo por archivo. Lo que importa es la seguridad:

- El token y las claves de operación van en `sessionStorage`, no en `localStorage`: duran lo que
  la pestaña y las claves se borran al cerrar sesión.
- La pestaña Operación sólo aparece con la clave de administración cargada. El control real lo
  hace la API, que exige `X-Admin-Key` en `/admin/*`.
- No se usa `innerHTML`: lo que viene de la API se escribe como texto.
- El saldo de bienvenida lo fija la API (`SALDO_BIENVENIDA_CENTAVOS`), no el cliente.
- Cada operación lleva su `Idempotency-Key`: si la red se corta después de liquidar, reintentar
  no cobra dos veces. Un `202` del proveedor se muestra como "en revisión".

## 8. Estado

El state vive en S3 (`ledger-tfstate-<cuenta>-us-east-1-an`, key `tp3/terraform.tfstate`). Tiene
que ser remoto porque Deploy y Destroy corren en GitHub Actions y un state local se perdería al
terminar cada job.

- Lo crea `make init` con la AWS CLI, porque el backend lo necesita antes del `init` y no puede
  ser un recurso del mismo state. Por eso `backend.tf` no tiene `bucket`.
- El lock es `use_lockfile = true`, el nativo del backend S3. El esquema de la clase, S3 más una
  tabla de DynamoDB, quedó deprecado en Terraform 1.11 a favor de éste.
- El `plan` de los pull requests corre con `-lock=false`: no escribe el state.
- Destroy borra el bucket al final, sólo si el state quedó vacío; si el destroy falló, queda para
  reintentar.
- Los buckets del proyecto también los crea la CLI (ver
  [Buckets de S3](README.md#buckets-de-s3-los-crea-la-aws-cli) en el README).

## 9. Trade-offs

- **Un solo NAT.** Si cae su AZ, la API de la otra sigue atendiendo, pero no llega al proveedor,
  al webhook ni a las claves de Cognito (una tarea nueva no puede validar sesiones hasta que
  vuelva).
- **Siete interface endpoints**, unos USD 0,14 por hora, tres veces el NAT: es el precio de que
  el archivador, el único que borra datos del ledger, no tenga salida a Internet.
- **La API en HTTP.** Sin dominio no hay certificado: el token y las claves de operación viajan
  en claro hasta el NLB. El login sí es HTTPS de punta a punta.
- **Los secretos están en el state**, porque los genera Terraform. El bucket del state es
  privado y cifrado.
- **`LabRole` para todo.** Las tareas, el agente y la Lambda tienen los permisos del rol del
  laboratorio, así que la separación de custodios de la clave KMS no es real; en una cuenta
  propia cada servicio tendría su rol.
- **Buckets fuera del state.** Terraform no se entera si alguien borra uno a mano, y el nombre se
  arma en el script y en los módulos (el `Makefile` define `PROJECT` una vez para los dos).
- **RDS Multi-AZ**, aunque duplique el primer apply; `db_multi_az = false` lo saca para probar.
  La base no sobrevive a Destroy (sin snapshot final ni deletion protection).
- **Sin X-Ray, Performance Insights ni VPC Flow Logs**: el usuario del laboratorio no puede leer
  los dos primeros, y `LabRole` no puede entregar Flow Logs a CloudWatch.
- tfsec marca estas decisiones; cada una está ignorada en el código con el motivo al lado.

## 10. Correcciones de entregas anteriores

| Entrega | Corrección | Qué se hizo |
|---|---|---|
| TP1 | Avanzar con la propuesta 2, el núcleo transaccional elástico | Es este sistema |
| TP1 | Un sistema financiero asíncrono obliga a lidiar con concurrencia y consistencia eventual | La concurrencia se resuelve en la base: bloqueo de fila en orden, idempotencia por restricción única y transacciones que escriben asientos y saldos juntos ([Datos](#4-datos)). La liquidación es síncrona porque hay una persona esperando el pago; lo asíncrono es lo que depende de afuera: los egresos que el proveedor no confirma se resuelven por consulta, y el archivado corre aparte |
| TP2 | Por la naturaleza de la aplicación, usar un network load balancer y no un application load balancer | NLB con targets `ip` ([Entrada: por qué un NLB](#entrada-por-qué-un-nlb)). La API escala por CPU, porque la métrica de requests por target es sólo del ALB |

## 11. Requisitos del TP

| Requisito | Dónde |
|---|---|
| ≥ 1 módulo externo | `terraform-aws-modules/vpc/aws ~> 6.7` en [`network.tf`](terraform/organization/network.tf) |
| ≥ 1 módulo propio | `security_groups`, `storage`, `website`, `auth`, `database`, `load_balancer`, `capacity`, `service` |
| ≥ 4 funciones | Más de 20 (ver el [README](README.md#funciones-utilizadas)) |
| ≥ 3 meta-argumentos | `for_each`, `count`, `depends_on` y `lifecycle` (ver el [README](README.md#meta-argumentos-utilizados)) |
| Variables y outputs | [`variables.tf`](terraform/organization/variables.tf) y [`outputs.tf`](terraform/organization/outputs.tf) |
| Pipeline (init, validate, plan, apply) | Workflows **Terraform** y **Deploy** en [`.github/workflows`](.github/workflows) |
| Diagrama actualizado | [`img/arquitectura.png`](img/arquitectura.png) (fuente en [`img/arquitectura.drawio`](img/arquitectura.drawio)) |
| Correcciones previas | [Sección 10](#10-correcciones-de-entregas-anteriores) |
