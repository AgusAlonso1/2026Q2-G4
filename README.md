# Núcleo Transaccional Elástico — TP3 (Terraform)

Cloud Computing · ITBA · 2026Q2 · Grupo 4

TODO: una línea de qué es el sistema y link a la arquitectura.

## Arquitectura

TODO: diagrama actualizado (`docs/arquitectura.png`), alineado con este código.

## Estructura

```
terraform/
├── organization/      # composición raíz: acá se corre terraform
└── modules/           # módulos propios
    ├── security_groups/
    ├── storage/       # bucket privado: archivo del ledger y binarios
    ├── website/       # bucket público: frontend
    ├── database/
    ├── load_balancer/
    ├── auth/          # Cognito + API Gateway + Lambda: callback HTTPS del login
    └── service/       # launch template + ASG, usado para API y archivador
apps/
├── web/               # frontend estático (lo sube modules/website)
└── callback/          # Lambda del callback de Cognito (JavaScript, sin dependencias ni build)
artifacts/             # binarios que se suben a S3
```

## Paso a paso

TODO: requisitos (Terraform >= 1.9, AWS CLI, credenciales de AWS Academy), y los
comandos en orden: `make init`, `make plan`, `make apply`, cómo probar la API y
`make destroy`.

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

Para cerrar sesión, el frontend borra los tokens y redirige a `logout_url`. Cognito vuelve por
`/logout` de API Gateway, porque tampoco acepta logout URLs HTTP.

`config.json` lo genera Terraform (`api_url`, `login_url`, `logout_url`), así que no va en
`apps/web/`. Los datos para la API (`issuer`, `client_id`) y el `user_pool_id` para crear
usuarios por CLI están en `terraform output cognito`.

## Pipeline

`.github/workflows/terraform.yml` corre en cada pull request:

1. `validate`: `fmt -check`, `init -backend=false` y `validate`. No necesita AWS.
2. `plan`: `init` y `plan` contra la cuenta de AWS Academy. Usa los secrets
   `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` y `AWS_SESSION_TOKEN`, que hay que
   renovar con cada sesión del laboratorio.

Los workflows de la cátedra (`terraform-format`, `terraform-docs`, `tfsec`) corren en
paralelo. `terraform-docs` regenera los README de `terraform/modules/` y hace commit en
la rama del PR: hacer `git pull` antes de seguir pusheando.

## Estado

TODO: por qué el state es local y qué implica.
