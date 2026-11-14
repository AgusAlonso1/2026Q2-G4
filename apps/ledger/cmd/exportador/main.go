package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ledger/db"
	"ledger/internal/avisos"
	"ledger/internal/cola"
	"ledger/internal/config"
	"ledger/internal/ledger"
	"ledger/internal/objetos"
	"ledger/internal/pg"
)

const plazoApagado = 25 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := correr(); err != nil {
		slog.Error("el exportador termino con error", "error", err)
		os.Exit(1)
	}
}

func correr() error {
	cfg, err := config.Cargar()
	if err != nil {
		return err
	}
	if err := cfg.ValidarExportador(); err != nil {
		return err
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancelar()

	arranque, cancelarArranque := context.WithTimeout(ctx, 30*time.Second)
	defer cancelarArranque()

	pool, err := pg.Abrir(arranque, cfg.DBURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	aplicadas, err := db.Aplicar(arranque, pool)
	if err != nil {
		return err
	}
	if len(aplicadas) > 0 {
		slog.Info("migraciones aplicadas", "versiones", aplicadas)
	}

	alm, err := objetos.Desde(arranque, cfg.Region, cfg.Bucket, cfg.ObjetosDir)
	if err != nil {
		return err
	}

	titulares, err := avisos.Nuevo(arranque, cfg.Region, cfg.TitularesTopicARN)
	if err != nil {
		return err
	}
	if !titulares.Habilitado() {
		slog.Warn("sin TITULARES_TOPIC_ARN: no se avisa a los titulares por mail")
	}

	exportaciones, err := cola.Nueva(arranque, cfg.Region, cfg.ExportacionesCola)
	if err != nil {
		return err
	}
	dlq, err := cola.Nueva(arranque, cfg.Region, cfg.ExportacionesDLQ)
	if err != nil {
		return err
	}

	svc, err := ledger.Nuevo(arranque, pool, nil, alm, nil, cfg.ConciliacionIntervalo, "")
	if err != nil {
		return err
	}
	svc.ConAvisos(titulares, cfg.AppURL).
		ConCola(exportaciones, dlq, cfg.ExportacionesIntentos, cfg.ExportacionesVisibilidad)

	slog.Info("exportador en marcha", "cola", exportaciones.URL(), "dlq", dlq.URL(),
		"consumidores", cfg.ExportacionesConsumidores, "intentos", cfg.ExportacionesIntentos,
		"visibilidad", cfg.ExportacionesVisibilidad.String())

	atendiendo := make(chan struct{})
	go func() {
		defer close(atendiendo)
		svc.AtenderExportaciones(ctx, cfg.ExportacionesConsumidores)
	}()

	select {
	case <-atendiendo:
		return nil
	case <-ctx.Done():
	}

	select {
	case <-atendiendo:
		slog.Info("exportador detenido")
	case <-time.After(plazoApagado):
		slog.Warn("exportador detenido con exportaciones en curso: vuelven por la cola al vencer su visibilidad")
	}
	return nil
}
