package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"harvester/internal/crawler"
	"harvester/internal/storage"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	// Usamos un archivo local harvester.db para guardar los datos
	dbPath := "harvester.db"
	store, err := storage.NewStorage(dbPath)
	if err != nil {
		log.Fatalf("Error crítico al inicializar almacenamiento SQLite: %v", err)
	}
	defer store.Close()

	command := os.Args[1]

	switch command {
	case "index":
		if len(os.Args) < 3 {
			fmt.Println("⚠️  Error: Debes proveer al menos una URL para indexar.")
			fmt.Println("Ejemplo: go run cmd/harvester/main.go index https://go.dev/doc/ https://go.dev/blog/")
			return
		}
		urls := os.Args[2:]
		runIndexing(store, urls)

	case "search":
		if len(os.Args) < 3 {
			fmt.Println("⚠️  Error: Debes ingresar un término de búsqueda.")
			fmt.Println("Ejemplo: go run cmd/harvester/main.go search \"worker pool\"")
			return
		}
		// Unir todos los argumentos restantes para permitir búsquedas con espacios sin comillas obligatorias
		query := strings.Join(os.Args[2:], " ")
		runSearch(store, query)

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println()
	fmt.Println("=======================================================")
	fmt.Println("🔥  AI Context Harvester - CLI (Fase 1)")
	fmt.Println("=======================================================")
	fmt.Println("Uso:")
	fmt.Println("  go run cmd/harvester/main.go index <url1> <url2> ...   Indexa concurrentemente las URLs")
	fmt.Println("  go run cmd/harvester/main.go search <término>          Busca en el índice FTS5 usando BM25")
	fmt.Println("=======================================================")
	fmt.Println()
}

func runIndexing(store *storage.Storage, urls []string) {
	fmt.Printf("\n🚀 Iniciando descarga concurrente de %d URL(s) con 3 workers...\n", len(urls))
	
	c := crawler.NewCrawler(3)
	
	// Timeout de 2 minutos máximo para toda la operación de descarga masiva
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startTime := time.Now()
	resultsChan := c.Crawl(ctx, urls)

	successCount := 0
	errorCount := 0

	for res := range resultsChan {
		if res.Err != nil {
			fmt.Printf("❌  [ERROR] %s -> %v\n", res.URL, res.Err)
			errorCount++
			continue
		}

		// Guardar en base de datos de manera atómica (metadatos + FTS5)
		docID, err := store.SaveDocument(res.URL, res.Title, res.Content)
		if err != nil {
			fmt.Printf("❌  [DB ERROR] Error al guardar %s en SQLite: %v\n", res.URL, err)
			errorCount++
			continue
		}

		fmt.Printf("✅  [INDEXADO] ID #%d | %s (%s)\n", docID, res.Title, res.URL)
		successCount++
	}

	fmt.Printf("\n✨ Ingesta concurrente finalizada en %v\n", time.Since(startTime).Round(time.Millisecond))
	fmt.Printf("📊 Resumen: %d exitosas, %d fallidas. Datos persistidos en 'harvester.db'\n\n", successCount, errorCount)
}

func runSearch(store *storage.Storage, query string) {
	fmt.Printf("\n🔍 Buscando por: \"%s\" (Clasificado por SQLite FTS5 BM25)\n", query)
	fmt.Println(strings.Repeat("=", 65))

	// Buscamos hasta las 5 mejores coincidencias
	results, err := store.Search(query, 5)
	if err != nil {
		log.Fatalf("Error ejecutando consulta en FTS5: %v", err)
	}

	if len(results) == 0 {
		fmt.Println("ℹ️  No se encontraron coincidencias para la consulta dada.")
		return
	}

	fmt.Printf("🎉 Se encontraron %d resultados altamente relevantes:\n\n", len(results))
	for i, res := range results {
		fmt.Printf("[%d] 🏆 BM25 Score: %.4f | %s\n", i+1, res.Rank, res.Title)
		fmt.Printf("    🔗 Fuente: %s\n", res.URL)
		fmt.Printf("    📝 Snippet: %s\n", res.Snippet)
		fmt.Println(strings.Repeat("-", 65))
	}
}
