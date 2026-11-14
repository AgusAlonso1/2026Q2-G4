# waf

Web ACL de AWS WAF asociada al user pool de Cognito: filtra los pedidos a la Hosted UI y a los
endpoints de OAuth antes de que lleguen al login. No toca el camino de los pagos, que entran
por el NLB.

- **Límite por IP**: bloquea una IP que pasa `rate_limit` pedidos en 5 minutos (fuerza bruta,
  credential stuffing).
- **Reglas administradas de AWS**: IPs con mala reputación, entradas maliciosas conocidas y el
  conjunto común (OWASP).
- **Logs** en CloudWatch, cifrados con la clave del proyecto, sin las cabeceras `cookie` ni
  `authorization`.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
