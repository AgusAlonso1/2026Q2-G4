# auth

Login con Cognito mediante el workaround de la cátedra. El frontend se sirve por HTTP desde S3,
y Cognito sólo acepta callbacks HTTPS. Por eso el callback es una ruta de API Gateway que
invoca una Lambda: la Lambda canjea el `code` por tokens y redirige al frontend con los
tokens en el fragment (`#access_token=…`).

```
frontend (HTTP) → login_url (Hosted UI) → GET <api>/callback?code&state
                → Lambda: POST /oauth2/token → 302 <frontend>/#access_token=…&state=…
frontend → logout_url (Hosted UI) → GET <api>/logout → 302 <frontend>/
```

- El app client es **confidencial**: el secret está en Secrets Manager (`<prefijo>/callback`) y
  la Lambda sólo tiene su ARN. Lo lee una vez por instancia, en el primer login.
- No se le entrega refresh token al frontend. Cuando el access token vence, el frontend vuelve
  a `login_url`, y si la sesión de la Hosted UI sigue viva, el usuario entra sin tipear nada.
- `/logout` existe porque Cognito también exige HTTPS en las logout URLs. Sin ese paso, la
  cookie de la Hosted UI sigue viva y no se puede cambiar de usuario.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
