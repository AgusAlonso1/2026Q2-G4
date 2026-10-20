# storage

Bucket S3 del proyecto: guarda los meses archivados del ledger. Se accede por el gateway
endpoint, sin pasar por el NAT. Se cifra con la clave KMS del proyecto (SSE-KMS) y su
política rechaza los pedidos que no usan HTTPS.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
