# Arquitectura — Núcleo Transaccional Elástico

Este documento describe la arquitectura que despliega este repositorio y los trade-offs de esta
iteración. El código Terraform es la fuente de verdad; acá se resume la intención. Cómo
desplegarlo está en el [README](README.md).

## 1. Contexto

El sistema es el núcleo transaccional de un PSP de una sola empresa. Cada usuario tiene una
cuenta virtual con la que transfiere a otras cuentas del PSP, ingresa dinero desde su banco y
lo retira hacia afuera a través de un proveedor de infraestructura financiera. Cada operación
se liquida dentro del mismo request, de forma idempotente, contra un ledger de partida doble.
Lo único diferido es el archivado de los meses viejos a S3, que ningún usuario pide.

Restricciones que guían el diseño:

- **AWS Academy**: no se pueden crear usuarios ni roles de IAM. Las instancias usan
  `LabInstanceProfile` y las tareas de ECS y la Lambda usan `LabRole`. Sólo está habilitada
  `us-east-1`.
- **Reproducible desde el pipeline**: todo vive en código y se despliega y destruye desde
  GitHub Actions, en cualquier cuenta, sin editar nada. Los nombres que tienen que ser únicos
  llevan el ID de la cuenta.
- **Costo acotado**: un solo NAT, instancias `t3.micro` y el archivador apagado fuera de su
  ventana.

## 2. Red

VPC `10.0.0.0/16` con el módulo `terraform-aws-modules/vpc/aws`, dos AZ y cuatro capas:

| Capa | CIDR (AZ a, AZ b) | Salida | Qué corre |
|---|---|---|---|
| `public` | `10.0.0.0/24`, `10.0.1.0/24` | IGW | NLB, NAT |
| `app` | `10.0.10.0/24`, `10.0.11.0/24` | NAT, gateway endpoint de S3 | Instancias y tareas de la API, ENIs de los interface endpoints |
| `archive` | `10.0.12.0/24`, `10.0.13.0/24` | Sólo el gateway endpoint de S3 | Instancias y tareas del archivador |
| `data` | `10.0.20.0/24`, `10.0.21.0/24` | Ninguna | RDS (primaria y standby) |

El aislamiento entre capas lo imponen los security groups, que se referencian entre sí:

| SG | Entra | Sale |
|---|---|---|
| `nlb` | 80 desde Internet | La API (puerto de la app) |
| `app` | Puerto de la app desde `nlb` | `data` (5432), S3 (443), Internet por el NAT (443 y 80: proveedor y webhook) |
| `archive` | Nada | `data` (5432), S3 (443) |
| `data` | 5432 desde `app` y `archive` | Nada |
| `host` | Nada | `endpoints` (443), S3 (443) |
| `endpoints` | 443 desde `host` | Nada |

Las instancias (`host`) no aceptan ninguna conexión entrante: no hay SSH.

## 3. Cómputo

La API y el archivador corren en **ECS con launch type EC2**, desde la misma imagen (cada
servicio elige `/api` o `/archivador`). Cada servicio tiene su propio ASG, en sus propias
subredes:

| | API | Archivador |
|---|---|---|
| Subredes (instancias y tareas) | `app`, con NAT | `archive`, sin ruta a Internet |
| Tareas | 2 a 6, escalan por CPU (60 %) | 0, y 1 durante la ventana de archivado |
| Instancias | Las que hagan falta para esas tareas | 0 fuera de la ventana |
| Tráfico | NLB → IP de cada tarea | Ninguno: sólo sale a la base y a S3 |

Hay **dos niveles de escalado**: el servicio decide cuántas tareas quiere (por CPU o por
horario) y el capacity provider agrega o apaga instancias para alojarlas. Con red `awsvpc`
cada tarea tiene su propia ENI y su security group, y una t3.micro aloja una sola tarea.

El agente de ECS de cada instancia necesita hablar con ECS, ECR y CloudWatch Logs. Como las
subredes de `archive` no tienen salida, lo hace por **interface endpoints** (`ecs`,
`ecs-agent`, `ecs-telemetry`, `ecr.api`, `ecr.dkr`, `logs`, `secretsmanager`); las capas de la
imagen vienen de S3 por el gateway endpoint. Las ENIs de los endpoints están en las subredes de `app`, pero se
llega a ellas desde cualquier subred de la VPC: lo que limita quién entra es su security group,
que sólo acepta a los hosts de ECS. Las instancias de la API usan los mismos endpoints, así que
el NAT sólo lo usan las tareas de la API (proveedor, webhook y claves de Cognito).

Todo el tráfico de la VPC hacia S3 pasa por el gateway endpoint, también el de las subredes de
`app`. Su política sólo deja llegar al bucket del proyecto y al de las capas de ECR: una tarea
comprometida no puede copiar datos a un bucket de otra cuenta. El security group no alcanza
para eso, porque la prefix list de S3 incluye todos los buckets. El bucket, por su lado, rechaza
cualquier pedido que no venga por HTTPS.

Los interface endpoints, en cambio, sólo aceptan credenciales de la propia cuenta. Lo que pasa
por ellos lo firma el rol de las instancias, así que el agente, la descarga de la imagen y los
logs no cambian, pero nadie puede usarlos con credenciales de otra cuenta para mandar datos
afuera. En S3 esa condición no sirve: las capas de ECR se bajan con URLs que firma AWS, no la
cuenta, y por eso ahí la política lista los buckets.

Un deploy con cambios en el backend sube una imagen nueva y crea una revisión de cada task
definition. ECS levanta las tareas nuevas antes de bajar las viejas: puede llegar al doble de
tareas y nunca baja de la mitad. Una tarea nueva de la API recibe tráfico recién cuando pasa el
health check del NLB, y si las nuevas no arrancan, el circuit breaker vuelve solo a la revisión
anterior. El tope de conexiones a la base de cada tarea se calcula para ese doble, así que un
deploy no agota las conexiones de RDS.

El cluster tiene **Container Insights**: CPU, memoria y red por servicio y por tarea en
CloudWatch, sin instalar nada en las instancias.

**Escaneo de la imagen.** El repositorio de ECR tiene `scan_on_push`: cada imagen que sube
`make imagen` se compara contra la base de vulnerabilidades conocidas (CVE) y el resultado
queda en la consola de ECR (`aws ecr describe-image-scan-findings`). Es el escaneo básico,
sin costo, y alcanza para esta imagen: la base es `distroless/static`, sin shell ni gestor de
paquetes, así que lo que podría aparecer está en las pocas librerías del sistema. El escaneo
avanzado (Amazon Inspector) revisa además las dependencias de Go, pero se cobra por imagen.
No bloquea el deploy: informa, y como los tags son inmutables cada versión queda escaneada
una sola vez, la vez que se sube.

## 4. Secretos y cifrado

Las claves que usan los binarios (la `DB_URL` con la password de la base, la de admin, la del
mock, la del proveedor y la del webhook) no van en la task definition, donde cualquiera con
acceso a la consola o a `ecs:DescribeTaskDefinition` las vería. Terraform las genera
(`random_password`) y las guarda en **Secrets Manager**, un secreto por servicio con sólo lo
que ese servicio usa: `ledger/api` tiene las cinco, `ledger/archive` sólo la `DB_URL`.

La task definition guarda la referencia, no el valor: cada variable es un `valueFrom` con
`<arn del secreto>:<clave>::<version-id>`. El agente de ECS lee el secreto con el rol de
ejecución antes de arrancar el contenedor y se lo pasa como variable de entorno, así que el
código no cambia. El `version-id` hace que un valor nuevo genere una revisión nueva de la task
definition, y ECS reemplaza las tareas; sin él seguirían corriendo con el valor viejo. El
agente llega a Secrets Manager por su interface endpoint, porque las subredes de `archive` no
salen a Internet.

El client secret de Cognito está en un tercer secreto, `ledger/callback`. La Lambda del
callback sólo recibe su ARN y lo lee con el SDK una vez por instancia; si la lectura falla no
queda guardado y el próximo login reintenta. La Lambda no corre en la VPC, así que no necesita
endpoint.

Los secretos se crean con `recovery_window_in_days = 0`: Secrets Manager reserva por defecto
el nombre 30 días después de borrarlo, y el siguiente Deploy fallaría por nombre duplicado.

**Clave KMS propia.** Todo se cifra en reposo aun sin ella: con nada indicado, cada servicio
usa una clave que administra AWS (`aws/secretsmanager`, `aws/rds`, ...). Lo que esas claves
no permiten es *gestionarlas*: su política no se puede modificar, no se pueden deshabilitar y
la rotación no la decide uno. Para un PSP eso importa: PCI DSS (requisitos 3.6 y 3.7) pide
restringir el acceso a las claves criptográficas a la menor cantidad posible de custodios,
rotarlas y poder retirar una clave comprometida. Por eso hay una clave propia, `alias/ledger`
([`kms.tf`](terraform/organization/kms.tf)):

- **Qué cifra:** los secretos de Secrets Manager, el bucket del archivo (SSE-KMS con *bucket
  key*, una clave de datos por bucket en lugar de una llamada a KMS por objeto), el
  almacenamiento y los backups de RDS, las imágenes de ECR y los logs de ECS, de la Lambda y
  de API Gateway.
- **Quién la usa:** la política nombra tres principales: la cuenta (sin ese permiso la clave
  quedaría inadministrable), el `LabRole`, con sólo las operaciones de cifrado (nombrado
  explícitamente para no depender de lo que permita su política de IAM en el laboratorio) y
  CloudWatch Logs, limitado a los log groups de la cuenta.
- **Rotación:** automática, anual. KMS conserva las versiones anteriores, así que lo cifrado
  antes se sigue pudiendo leer.
- **Retiro:** deshabilitar la clave corta de una vez el acceso a todos esos datos. Al
  destruir, KMS la programa para borrarse en 7 días (el mínimo); el próximo Deploy crea otra.

El bucket del frontend queda con AES256: lo leen usuarios anónimos por el endpoint de website,
y un anónimo no puede tener permiso sobre una clave de KMS.

## 5. Login con Cognito

Cognito sólo acepta callbacks HTTPS y el frontend se sirve por HTTP desde S3, así que el
callback pasa por API Gateway y una Lambda:

1. El frontend lee `config.json` y redirige a `login_url` agregando `&state=<random>`.
2. La Hosted UI de Cognito vuelve a `https://<api-gateway>/callback?code=…&state=…`.
3. La Lambda canjea el `code` por tokens (con el client secret, que lee de Secrets Manager,
   `ledger/callback`) y redirige
   a `<frontend>/#access_token=…&id_token=…&expires_in=…&state=…`.
4. El frontend valida el `state`, guarda los tokens y limpia la URL.
5. El frontend llama a la API con `Authorization: Bearer <access_token>`. La API valida la
   firma contra `<issuer>/.well-known/jwks.json` y comprueba `client_id` y `token_use=access`.
   Por CORS, sólo acepta pedidos del navegador que vengan de `website_url` (`CORS_ORIGENES`).
6. La primera vez, `GET /sesiones/actual` devuelve `cuenta_id: null` y el frontend crea la cuenta
   con `POST /cuentas`. La cuenta queda ligada al `sub` del usuario: cada usuario tiene una sola,
   y es la única que puede leer y operar (si no, 403).

Si el token vence (según `expires_in`) o la API responde 401, el frontend borra los tokens y
vuelve a la pantalla de ingreso. Para cerrar sesión, borra los tokens y redirige a
`logout_url`. Cognito vuelve por `/logout` de API Gateway, porque tampoco acepta logout URLs
HTTP.

`config.json` lo genera Terraform (`api_url`, `login_url`, `logout_url`), así que no va en
`apps/web/`. Los datos para la API (`issuer`, `client_id`) y el `user_pool_id` para crear
usuarios por CLI están en `terraform output cognito`.

## 6. Estado

El state vive en S3, en `ledger-tfstate-<cuenta>-us-east-1-an`, con la key
`tp3/terraform.tfstate`. Tiene que ser remoto porque Deploy y Destroy corren en GitHub Actions:
con state local se perdería al terminar cada job, el siguiente apply intentaría crear todo de
nuevo y Destroy no tendría qué destruir.

- **Lo crea `make init`, no Terraform.** El backend necesita el bucket antes del `init`, así que
  no puede ser un recurso del mismo state. `make init` lo crea con la AWS CLI
  ([`scripts/state-bucket.sh`](scripts/state-bucket.sh)) si no existe (versionado, cifrado y
  sin acceso público, que S3 aplica por defecto) y le pasa el nombre al backend con
  `-backend-config`. Por eso [`backend.tf`](terraform/organization/backend.tf) no tiene
  `bucket`.
- **El nombre lleva la cuenta**, en el namespace regional de la cuenta (`-an`), igual que los
  otros dos buckets: el mismo código despliega en la cuenta del grupo y en la de la cátedra sin
  editar nada, y nadie más puede ocupar ese nombre.
- **Lock:** `use_lockfile = true`, el lock nativo del backend S3 (Terraform >= 1.10). No hace
  falta una tabla de DynamoDB. El `plan` de los pull requests corre con `-lock=false`: no
  escribe el state, y así no espera a un deploy en curso.
- **Destroy lo borra al final**, con todas sus versiones, pero sólo si `terraform state list`
  quedó vacío: si el destroy falló a la mitad, el bucket queda para poder reintentar. El
  próximo `make init` (Deploy o el plan de un pull request) lo vuelve a crear.

## 7. Trade-offs

- **Un solo NAT**, en la primera AZ. Ahorra el costo de uno por AZ; si esa AZ se cae, las
  tareas de la API de la otra siguen atendiendo y llegando a la base, pero no al proveedor ni
  al webhook. Los servicios de AWS no dependen del NAT: van por los endpoints.
- **NLB en HTTP, sin TLS.** El laboratorio no tiene un dominio propio para emitir un
  certificado. El token viaja en claro entre el navegador y el NLB; el login, en cambio, es
  HTTPS de punta a punta (Hosted UI y API Gateway).
- **Los secretos siguen en el state de Terraform**, porque los genera Terraform. El state está
  en un bucket privado y cifrado.
- **`LabRole` para todo**: las tareas, el agente de ECS y la Lambda tienen los permisos del rol
  del laboratorio, no unos mínimos por componente. Por eso la separación de custodios de la
  clave KMS no es real: en una cuenta propia cada servicio tendría su rol y la política de la
  clave nombraría sólo a los que la necesitan.
- **RDS Multi-AZ** aunque duplique el tiempo del primer apply: la base es lo único con estado y
  sigue disponible si se cae una zona. `db_multi_az = false` la deja en una sola AZ para probar.
- **La base no sobrevive a Destroy**: sin deletion protection ni snapshot final, porque Destroy
  tiene que poder borrarla sin un apply previo. Mientras está desplegada guarda 7 días de backups
  automáticos, que permiten volver a cualquier punto de ese período (por ejemplo, después de un
  reinicio del ledger).
- **Sin X-Ray en la Lambda, Performance Insights en RDS ni VPC Flow Logs**, por el laboratorio:
  el usuario de AWS Academy no puede leer las trazas de X-Ray ni las métricas de Performance
  Insights (los dos se pueden activar, pero nadie vería lo que juntan), y la trust policy de
  `LabRole` no incluye a `vpc-flow-logs.amazonaws.com`, así que los Flow Logs sólo podrían ir a
  S3, a un bucket aparte.
- **Sin autenticación IAM en RDS**: la API entra con la credencial de Secrets Manager, y
  habilitarla sólo sumaría una vía de login que nadie usa.
- tfsec marca estas decisiones; cada una está ignorada en el código con el motivo al lado.

## 8. Requisitos del TP

| Requisito | Dónde en este repo |
|---|---|
| ≥ 1 módulo externo | `terraform-aws-modules/vpc/aws ~> 6.7` en [`network.tf`](terraform/organization/network.tf) |
| ≥ 1 módulo propio | `security_groups`, `storage`, `website`, `auth`, `database`, `load_balancer`, `capacity`, `service` (8) |
| ≥ 4 funciones | `cidrsubnet`, `slice`, `range`, `substr`, `floor`, `sum`, `coalesce`, `concat`, `replace`, `try`, `format`, `jsonencode`, `toset`, `fileset`, `regex`, `filemd5`, `urlencode`, `templatefile`, `base64encode`, `sort`, `nonsensitive`, entre otras (ver el [README](README.md#funciones-utilizadas)) |
| ≥ 3 meta-argumentos | `for_each`, `count`, `depends_on` y `lifecycle` (ver el [README](README.md#meta-argumentos-utilizados)) |
| Variables y outputs | [`variables.tf`](terraform/organization/variables.tf) parametriza red, base, cómputo, proveedor y ventana del archivador; [`outputs.tf`](terraform/organization/outputs.tf) expone las URL y lo necesario para operar |
| Pipeline (init, validate, plan, apply) | Workflows **Terraform** y **Deploy** en [`.github/workflows`](.github/workflows) |
