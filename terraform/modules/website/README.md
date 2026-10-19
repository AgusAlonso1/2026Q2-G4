# website

Bucket S3 público con website hosting para el frontend estático. Se sirve por HTTP, como
pide el workaround de Cognito de la cátedra: el HTTPS del callback lo pone API Gateway.
Sube cada archivo de `site_dir` con su `Content-Type`, y sólo publica las extensiones
declaradas en `content_types`. Además publica `config.json` a partir de `runtime_config`:
la URL de la API y las de login y logout de Cognito, que no existen hasta el apply.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
