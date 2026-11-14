# analytics

Consultas SQL con Athena sobre el archivo del ledger, sin tocar la base transaccional. El
archivador deja cada mes en `transaction-history/periodo=AAAA-MM/asientos.ndjson.gz`; el módulo
lo cataloga en Glue como la tabla `asientos`, particionada por `periodo`.

- **Particiones por proyección.** La tabla no guarda particiones en el catálogo: Athena las
  calcula desde `history_start` hasta el mes actual. Un mes recién archivado se puede consultar
  enseguida, sin crawler (que necesitaría un rol propio) ni `MSCK REPAIR TABLE`.
- **Workgroup con la configuración forzada.** Los resultados van a `athena-results/` del mismo
  bucket, cifrados con la clave del proyecto, y cada consulta tiene un tope de bytes escaneados.
  `storage` los expira a los 7 días; como el bucket está versionado, pasan a ser una versión
  anterior y se borran 30 días después.
- **Consultas guardadas** en el workgroup: `partida-doble`, `volumen-mensual`,
  `cuentas-mas-activas` y `movimientos-de-cuenta` (con `cuenta_id` como parámetro).

`creado_en` es un string RFC 3339; se convierte con `from_iso8601_timestamp(creado_en)`. El
`periodo` del JSON no es una columna: es la partición, en formato `AAAA-MM`.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
