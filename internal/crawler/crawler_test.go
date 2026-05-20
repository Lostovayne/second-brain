package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCrawler_Crawl(t *testing.T) {
	// 1. Crear un servidor de prueba HTTP local
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `
			<!DOCTYPE html>
			<html>
			<head>
				<title>Página de Prueba Go</title>
			</head>
			<body>
				<nav>
					<a href="/home">Inicio</a> | <a href="/about">Nosotros</a>
				</nav>
				
				<main>
					<h1>Aprender Concurrencia en Go</h1>
					<p>Go hace que la concurrencia sea fácil de usar mediante goroutines.</p>
					
					<pre><code>package main
import "fmt"
func main() {
	go fmt.Println("Concurrente!")
}</code></pre>
				</main>

				<footer>
					<p>© 2026 - Mi footer ignorado</p>
				</footer>
			</body>
			</html>
		`)
	}))
	defer server.Close()

	// 2. Inicializar el Crawler
	c := NewCrawler(2) // 2 workers
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 3. Lanzar la descarga concurrente hacia el servidor mock
	resultsChan := c.Crawl(ctx, []string{server.URL})

	var results []Result
	for res := range resultsChan {
		results = append(results, res)
	}

	// 4. Validar resultados
	if len(results) != 1 {
		t.Fatalf("Se esperaba 1 resultado, se obtuvieron: %d", len(results))
	}

	res := results[0]
	if res.Err != nil {
		t.Fatalf("El Crawler falló con error: %v", res.Err)
	}

	if res.Title != "Página de Prueba Go" {
		t.Errorf("Título incorrecto, esperado 'Página de Prueba Go', obtenido: '%s'", res.Title)
	}

	// 5. Validar filtrado de etiquetas molestas (nav, footer)
	if strings.Contains(res.Content, "Mi footer ignorado") {
		t.Error("El crawler debió ignorar el contenido dentro de <footer>")
	}
	if strings.Contains(res.Content, "Inicio | Nosotros") {
		t.Error("El crawler debió ignorar el contenido dentro de <nav>")
	}

	// 6. Validar conversión exitosa a Markdown
	if !strings.Contains(res.Content, "# Aprender Concurrencia en Go") {
		t.Error("No se parseó el <h1> a Markdown '#'")
	}
	if !strings.Contains(res.Content, "Go hace que la concurrencia sea fácil") {
		t.Error("No se recuperó el párrafo correctamente")
	}
	
	// Validar que se parseó el bloque <pre><code> a un bloque de código markdown
	expectedCodeBlock := "```go\npackage main\nimport \"fmt\"\nfunc main() {\n\tgo fmt.Println(\"Concurrente!\")\n}\n```"
	if !strings.Contains(res.Content, expectedCodeBlock) {
		t.Errorf("No se parseó el bloque de código correctamente.\nEsperado:\n%s\n\nObtenido:\n%s", expectedCodeBlock, res.Content)
	}
}
