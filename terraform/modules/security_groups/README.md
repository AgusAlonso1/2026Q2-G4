# security_groups

Un security group por capa, encadenados por referencia entre grupos: `alb ← internet`,
`app ← alb`, `data ← app, archive`. Sin SSH y con las NACLs por defecto.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
