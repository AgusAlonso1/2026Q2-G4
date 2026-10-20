# capacity

Instancias EC2 donde corren las tareas de un servicio de ECS: launch template con la AMI
optimizada para ECS, Auto Scaling group y capacity provider con *managed scaling*. El
capacity provider agrega instancias cuando hay tareas sin lugar y apaga las que quedan
vacías, así que el ASG sigue a las tareas y no tiene política de escalado propia.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
