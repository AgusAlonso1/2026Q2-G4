# load_balancer

Network Load Balancer en las subredes públicas, listener HTTP:80 y target group con
health check de aplicación sobre `/health`. Los targets son las IP de las tareas de
ECS (red `awsvpc`), no las instancias.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
