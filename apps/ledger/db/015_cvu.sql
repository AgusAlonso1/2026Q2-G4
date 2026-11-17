ALTER TABLE mock_movimientos
    ADD COLUMN IF NOT EXISTS entidad       TEXT,
    ADD COLUMN IF NOT EXISTS cvu_destino   TEXT,
    ADD COLUMN IF NOT EXISTS cbu_origen    TEXT,
    ADD COLUMN IF NOT EXISTS nombre_origen TEXT;

CREATE INDEX IF NOT EXISTS mock_movimientos_entidad_idx ON mock_movimientos (entidad, id);

ALTER TABLE movimientos_banco
    ADD COLUMN IF NOT EXISTS cvu_destino TEXT,
    ADD COLUMN IF NOT EXISTS cbu_origen  TEXT;

ALTER TABLE ordenes
    ADD COLUMN IF NOT EXISTS cbu_origen    TEXT,
    ADD COLUMN IF NOT EXISTS nombre_origen TEXT;

CREATE TABLE IF NOT EXISTS mock_alias (
    alias          TEXT PRIMARY KEY,
    cvu            TEXT        NOT NULL UNIQUE,
    entidad        TEXT        NOT NULL,
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE cuentas ADD COLUMN IF NOT EXISTS alias_publicado_en TEXT;

CREATE TABLE IF NOT EXISTS mock_entidades (
    entidad   TEXT PRIMARY KEY,
    billetera TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS mock_solicitudes (
    referencia      TEXT PRIMARY KEY,
    cvu_pagador     TEXT        NOT NULL,
    alias_pagador   TEXT,
    cvu_cobrador    TEXT        NOT NULL,
    alias_cobrador  TEXT,
    monto_centavos  BIGINT      NOT NULL CHECK (monto_centavos > 0),
    estado          TEXT        NOT NULL DEFAULT 'pendiente'
                    CHECK (estado IN ('pendiente', 'aceptada', 'rechazada')),
    motivo          TEXT,
    creada_en       TIMESTAMPTZ NOT NULL DEFAULT now(),
    resuelta_en     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS mock_solicitudes_pagador_idx ON mock_solicitudes (cvu_pagador, creada_en DESC);
CREATE INDEX IF NOT EXISTS mock_solicitudes_cobrador_idx ON mock_solicitudes (cvu_cobrador, creada_en DESC);
