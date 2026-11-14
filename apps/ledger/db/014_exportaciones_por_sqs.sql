ALTER TABLE solicitudes_exportacion ADD COLUMN IF NOT EXISTS tomada_por TEXT;
ALTER TABLE solicitudes_exportacion ADD COLUMN IF NOT EXISTS intentos INT NOT NULL DEFAULT 0;
ALTER TABLE solicitudes_exportacion ADD COLUMN IF NOT EXISTS encolada_en TIMESTAMPTZ;
ALTER TABLE solicitudes_exportacion ADD COLUMN IF NOT EXISTS reintentar_desde TIMESTAMPTZ;
