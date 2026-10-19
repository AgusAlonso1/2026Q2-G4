# Núcleo Transaccional Elástico — TP3 (Terraform)

Cloud Computing · ITBA · 2026Q2 · Grupo 4

TODO: una línea de qué es el sistema y link a la arquitectura.

TODO: diagrama actualizado (`docs/arquitectura.png`), alineado con este código.

TODO: flujo de datos en pasos numerados (frontend → login → API → base → archivador).

El despliegue se hace desde **GitHub Actions**: el workflow **Deploy** aplica todo el Terraform
en la cuenta de AWS cuyos secrets tenga el repositorio, y **Destroy** lo borra todo.

---

## Prerrequisitos

| Herramienta | Versión / nota |
|---|---|
| Terraform | `>= 1.10` (ver [`versions.tf`](terraform/organization/versions.tf)); el pipeline usa 1.16.6 |
| AWS CLI | v2 reciente (`--bucket-namespace` en `create-bucket`), sólo para correr desde una máquina local |
| Cuenta | AWS Academy: usa `LabRole` y `LabInstanceProfile`, que ya existen en el laboratorio |

Para desplegar desde GitHub Actions no hace falta nada instalado localmente: alcanza con los
secrets del paso 1.

---

## Integrantes

| Nombre | Legajo |
|---|---|
| TODO | |

---

## Pasos a seguir

### 1. Configurar secrets

En el repositorio, **Settings → Secrets and variables → Actions → New repository secret**:

| Secret | Para qué |
|---|---|
| `AWS_ACCESS_KEY_ID` | Access key del laboratorio de AWS Academy |
| `AWS_SECRET_ACCESS_KEY` | Secret access key del laboratorio |
| `AWS_SESSION_TOKEN` | Session token del laboratorio |

Los tres salen de **AWS Details → AWS CLI** en el laboratorio y vencen con la sesión: hay que
volver a cargarlos cada vez que se inicia el lab, antes de correr un workflow.

### 2. Workflows

| Workflow | Cuándo corre | Qué hace |
|---|---|---|
| **Terraform** | En cada pull request, o a mano | `fmt -check`, `init -backend=false` y `validate` (sin AWS); después `init` y `plan` contra la cuenta. El resultado del plan queda en el resumen del run |
| **Deploy** | A mano, escribiendo `deploy` | `make plan` y `make apply`. Las URL del despliegue quedan en el resumen del run |
| **Destroy** | A mano, escribiendo `destroy` | `terraform destroy` de toda la infraestructura y después el bucket del state |

Deploy y Destroy comparten un grupo de `concurrency`: nunca corren a la vez sobre el mismo state.

Los workflows de la cátedra (`terraform-format`, `terraform-docs`, `tfsec`) corren en paralelo
en cada pull request. `terraform-docs` regenera los README de `terraform/modules/` y hace commit
en la rama del PR: hacer `git pull` antes de seguir pusheando.

### 3. Desplegar

1. Con los secrets cargados: **Actions → Deploy → Run workflow** → rama `main` → escribir
   `deploy`. Mergear no despliega nada.
2. Esperar el run en verde. El primer apply tarda más que los siguientes: RDS Multi-AZ sola
   lleva varios minutos.
3. Abrir el resumen del run: tiene `website_url` (frontend), `api_url` y `login_url`.

### 4. Probar

TODO: `curl <api_url>/health`, crear un usuario en Cognito con
`aws cognito-idp admin-create-user --user-pool-id <terraform output cognito>` y entrar por
`website_url`.

### 5. Destruir al terminar

**Actions → Destroy → Run workflow** → escribir `destroy`.

Borra todos los recursos y, cuando el state quedó vacío, también el bucket del state (ver
[Estado](#estado)). No queda nada del TP en la cuenta.

### Desde una máquina local

Con las credenciales del laboratorio en `~/.aws/credentials` (o en variables de entorno), los
mismos pasos que el pipeline:

```bash
make init      # crea el bucket del state si no existe e inicializa terraform/organization
make plan      # deja el plan en terraform/organization/tfplan
make apply     # aplica ese plan
make destroy   # pide confirmación; borra también el bucket del state
```

`make help` lista el resto (`fmt`, `fmt-check`, `validate`, `clean`). Local y pipeline usan el
mismo state, así que lo desplegado desde un lado se ve y se destruye desde el otro.

---

## Módulos

| Módulo | Tipo | Qué crea |
|---|---|---|
| `terraform-aws-modules/vpc/aws` | externo | VPC, 8 subredes en 4 capas × 2 AZ, IGW, NAT, tablas de ruteo |
| `security_groups` | propio | Un SG por capa (`nlb`, `app`, `archive`, `data`) con reglas que referencian grupos, no rangos |
| `storage` | propio | TODO |
| `website` | propio | Bucket público con website hosting (HTTP), los archivos del frontend y su `config.json` |
| `auth` | propio | User pool de Cognito con Hosted UI, y API Gateway HTTP + Lambda que hacen de callback HTTPS (ver "Login con Cognito") |
| `database` | propio | RDS PostgreSQL Multi-AZ en la capa de datos, cifrada, sin acceso público |
| `load_balancer` | propio | NLB público en HTTP:80 con health check HTTP a `/health` y target group de la API |
| `service` | propio | TODO |

## Funciones

| Función | Dónde | Para qué |
|---|---|---|
| `cidrsubnet` | `organization/locals.tf` | Deriva los CIDR de cada capa desde el de la VPC |
| `slice` | `organization/locals.tf` | Toma las dos primeras AZ de la región |
| `range` | `organization/locals.tf` | Una subred por AZ dentro de cada capa |
| `substr` | `organization/locals.tf` | Saca la letra de la AZ para el nombre de la subred (`ledger-app-a`) |
| `concat` | `organization/network.tf` | Asocia el endpoint de S3 a las tablas de app y archivado |
| `try` | `modules/security_groups/main.tf` | Usa el SG de destino sólo si el `peer` de la regla es otro grupo |
| `replace` | `modules/security_groups/main.tf` | Arma el `Name` de cada regla desde su clave (`app_in_nlb` → `ledger-app-in-nlb`) |
| `format` | `modules/database/outputs.tf` | Arma la `DB_URL` de los binarios desde los atributos de la instancia |
| `fileset` | `modules/website/main.tf` | Lista los archivos del frontend para subir uno por uno |
| `regex`, `lower`, `contains`, `keys` | `modules/website/main.tf` | Saca la extensión y publica sólo los tipos conocidos |
| `jsonencode` | `modules/website/main.tf` | Arma la política de lectura pública |
| `urlencode`, `join` | `modules/auth/outputs.tf` | Arma las URL de login y logout de la Hosted UI |
| `toset` | `modules/auth/main.tf` | Una ruta de API Gateway por elemento (`/callback`, `/logout`) |
| TODO: `templatefile`, ... | | |

## Meta-argumentos

| Meta-argumento | Dónde | Para qué |
|---|---|---|
| `for_each` | `organization/main.tf` (`module "service"`) | Un ASG por servicio sin duplicar código |
| `for_each` | `modules/security_groups/main.tf` | Un SG por capa y una regla por fila de la tabla de reglas |
| `for_each` | `modules/auth/main.tf` | Las rutas `/callback` y `/logout` hacia la misma integración |
| `count` | `modules/website/main.tf` | Publica `config.json` sólo si hay `runtime_config` |
| `depends_on` | `modules/auth/main.tf` | El log group de la Lambda existe antes que ella, con retención y dentro del state |
| TODO: `lifecycle` | | |

## Login con Cognito

Workaround de la cátedra: Cognito sólo acepta callbacks HTTPS y el frontend se sirve por HTTP
desde S3.

1. El frontend lee `config.json` y redirige a `login_url` agregando `&state=<random>`.
2. La Hosted UI de Cognito vuelve a `https://<api-gateway>/callback?code=…&state=…`.
3. La Lambda canjea el `code` por tokens (con el client secret, que sólo tiene ella) y redirige
   a `<frontend>/#access_token=…&id_token=…&expires_in=…&state=…`.
4. El frontend valida el `state`, guarda los tokens y limpia la URL.
5. El frontend llama a la API con `Authorization: Bearer <access_token>`. La API valida la
   firma contra `<issuer>/.well-known/jwks.json` y comprueba `client_id` y `token_use=access`.
6. La primera vez, `GET /sesiones/actual` devuelve `cuenta_id: null` y el frontend crea la cuenta
   con `POST /cuentas`. La cuenta queda ligada al `sub` del usuario: cada usuario tiene una sola,
   y es la única que puede leer y operar (si no, 403).

Para cerrar sesión, el frontend borra los tokens y redirige a `logout_url`. Cognito vuelve por
`/logout` de API Gateway, porque tampoco acepta logout URLs HTTP.

`config.json` lo genera Terraform (`api_url`, `login_url`, `logout_url`), así que no va en
`apps/web/`. Los datos para la API (`issuer`, `client_id`) y el `user_pool_id` para crear
usuarios por CLI están en `terraform output cognito`.

## Estado

El state vive en S3, en `ledger-tfstate-<cuenta>-us-east-1-an`, con la key
`tp3/terraform.tfstate`. Tiene que ser remoto porque Deploy y Destroy corren en GitHub Actions: con state local se perdería al terminar cada job, el siguiente apply intentaría crear
todo de nuevo y Destroy no tendría qué destruir.

- **Lo crea `make init`, no Terraform.** El backend necesita el bucket antes del `init`, así que
  no puede ser un recurso del mismo state. `make init` lo crea con la AWS CLI
  ([`scripts/state-bucket.sh`](scripts/state-bucket.sh)) si no existe
  (versionado, cifrado y sin acceso público, que S3 aplica por defecto) y le pasa el nombre al
  backend con `-backend-config`. Por eso [`backend.tf`](terraform/organization/backend.tf) no
  tiene `bucket`.
- **El nombre lleva la cuenta**, en el namespace regional de la cuenta (`-an`), igual que los
  otros dos buckets: el mismo código despliega en la cuenta del grupo y en la de la cátedra sin
  editar nada, y nadie más puede ocupar ese nombre.
- **Lock:** `use_lockfile = true`, el lock nativo del backend S3 (Terraform >= 1.10). No hace
  falta una tabla de DynamoDB. El `plan` de los pull requests corre con `-lock=false`: no escribe el state, y así no
  espera a un deploy en curso.
- **Destroy lo borra al final**, con todas sus versiones, pero sólo si `terraform state list`
  quedó vacío: si el destroy falló a la mitad, el bucket queda para poder reintentar. El
  próximo `make init` (Deploy o el plan de un pull request) lo vuelve a crear.

---

## Mapa del repositorio

```
.
├── .github/workflows/     # Terraform (validate + plan), Deploy, Destroy y los de la cátedra
├── Makefile               # init (con el bucket del state), plan, apply, destroy, fmt, validate
├── scripts/
│   └── state-bucket.sh    # crea y borra el bucket del state
├── terraform/
│   ├── organization/      # composición raíz: acá se corre terraform
│   └── modules/           # módulos propios
│       ├── security_groups/
│       ├── storage/       # bucket privado: archivo del ledger
│       ├── website/       # bucket público: frontend
│       ├── database/
│       ├── load_balancer/
│       ├── auth/          # Cognito + API Gateway + Lambda: callback HTTPS del login
│       └── service/       # launch template + ASG, usado para API y archivador
└── apps/
    ├── ledger/            # backend en Go: binarios api y archivador, imagen con su Dockerfile
    ├── web/               # frontend estático (lo sube modules/website)
    └── callback/          # Lambda del callback de Cognito (JavaScript, sin dependencias ni build)
```
