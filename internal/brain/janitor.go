package brain

import (
	"context"
	"fmt"
	"os"
	"time"

	"harvester/internal/storage"
)

// Janitor corre en segundo plano realizando tareas de autolimpieza y autoaprendizaje en la base de datos.
type Janitor struct {
	store    *storage.Storage
	interval time.Duration
}

// NewJanitor inicializa un nuevo limpiador de cerebro periódico.
func NewJanitor(store *storage.Storage, interval time.Duration) *Janitor {
	return &Janitor{
		store:    store,
		interval: interval,
	}
}

// Start levanta el ciclo de ejecución asíncrono controlado por una Goroutine y un Ticker.
func (j *Janitor) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)

	go func() {
		defer ticker.Stop()
		fmt.Fprintf(os.Stderr, "🤖 [BRAIN JANITOR] Servicio de limpieza y deduplicación asíncrona inicializado (Intervalo: %v).\n", j.interval)
		
		for {
			select {
			case <-ctx.Done():
				fmt.Fprintf(os.Stderr, "🤖 [BRAIN JANITOR] Deteniendo servicio en segundo plano de forma segura.\n")
				return
			case <-ticker.C:
				j.RunCleanup()
			}
		}
	}()
}

// RunCleanup orquesta de forma atómica la ejecución de las tareas del janitor.
func (j *Janitor) RunCleanup() {
	// 1. Deduplicar observaciones redundantes usando similitud de tokens Jaccard
	deletedCount, err := j.store.Deduplicate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ [BRAIN JANITOR ERROR] Error en deduplicación: %v\n", err)
	} else if deletedCount > 0 {
		fmt.Fprintf(os.Stderr, "🤖 [BRAIN JANITOR] Se consolidaron y eliminaron %d observación(es) redundante(s).\n", deletedCount)
	}

	// 2. Escaneo semántico para auto-relacionar entidades
	linkedCount, err := j.store.AutoLink()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ [BRAIN JANITOR ERROR] Error en análisis semántico: %v\n", err)
	} else if linkedCount > 0 {
		fmt.Fprintf(os.Stderr, "🤖 [BRAIN JANITOR] El autoaprendizaje mapeó %d nueva(s) relación(es) semántica(s) en SQLite.\n", linkedCount)
	}
}
