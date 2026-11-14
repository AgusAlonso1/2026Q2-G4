output "workgroup_name" {
  description = "Workgroup de Athena: hay que elegirlo en el editor para usar su configuración y sus consultas guardadas."
  value       = aws_athena_workgroup.this.name
}

output "database_name" {
  description = "Base del catálogo de Glue."
  value       = aws_glue_catalog_database.this.name
}

output "table_name" {
  description = "Tabla de los asientos archivados."
  value       = aws_glue_catalog_table.asientos.name
}

output "results_location" {
  description = "Dónde escribe Athena los resultados de las consultas."
  value       = aws_athena_workgroup.this.configuration[0].result_configuration[0].output_location
}

output "named_query_ids" {
  description = "Ids de las consultas guardadas, por nombre."
  value       = { for nombre, q in aws_athena_named_query.this : nombre => q.id }
}
