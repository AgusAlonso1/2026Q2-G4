# service

Un servicio de ECS: task definition con red `awsvpc` (cada tarea tiene su ENI y su security
group), el servicio con su capacity provider y, si recibe tráfico, su target group. Escala
las tareas por CPU, por horario o de las dos formas; las instancias las pone el módulo
`capacity`. Los logs del contenedor van a CloudWatch Logs.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
