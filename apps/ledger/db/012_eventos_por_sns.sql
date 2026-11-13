-- Decisión 0023: la entrega del webhook pasa a SQS y una Lambda, así que la API ya no registra
-- cada entrega. El receptor descarta los avisos repetidos por evento_id (SQS entrega al menos
-- una vez); los avisos sin evento_id o con firma inválida quedan en NULL y no se deduplican.
DROP TABLE IF EXISTS entregas_webhook;

ALTER TABLE avisos_recibidos ADD COLUMN evento_id TEXT UNIQUE;
