package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"harvester/internal/brain"
	"harvester/internal/mcp"
	"harvester/internal/storage"
)

func main() {
	// 1. Configurar logs para salir estrictamente a Stderr.
	// Esto es sumamente importante para evitar corromper la salida stdout, 
	// que es el canal exclusivo de comunicación JSON-RPC del protocolo MCP.
	log.SetOutput(os.Stderr)
	log.SetPrefix("🤖 [MCP MAIN] ")

	dbPath := "harvester.db"
	store, err := storage.NewStorage(dbPath)
	if err != nil {
		log.Fatalf("Error crítico al inicializar almacenamiento SQLite: %v", err)
	}
	defer func() {
		log.Println("Cerrando la conexión de base de datos SQLite.")
		store.Close()
	}()

	// 2. Crear un contexto cancelable por señales del sistema operativo para cierre gracioso
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. Inicializar e iniciar el Brain Janitor en segundo plano (limpieza y deduplicación)
	// Usamos un intervalo de 5 minutos por defecto.
	janitor := brain.NewJanitor(store, 5*time.Minute)
	janitor.Start(ctx)

	// 4. Inicializar el Servidor MCP sobre Stdio (os.Stdin y os.Stdout)
	server := mcp.NewServer(store, os.Stdin, os.Stdout)
	
	log.Println("Servidor MCP iniciado exitosamente. Escuchando peticiones JSON-RPC sobre Stdio...")

	// Arrancamos el servidor en una goroutine independiente para reaccionar a señales de parada
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start(ctx)
	}()

	// Esperar fin por señal del sistema o fin de lectura de stdio
	select {
	case <-ctx.Done():
		log.Println("Recibida señal de detención (SIGINT/SIGTERM). Apagando servidor MCP...")
		// Permitimos que los aplazamientos (defers) cierren la base de datos y janitor se apague.
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Error crítico en el ciclo de ejecución del servidor MCP: %v", err)
		}
	}
	
	log.Println("Servidor MCP finalizado con éxito de forma segura.")
}
