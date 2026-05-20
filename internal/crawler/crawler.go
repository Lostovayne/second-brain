package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// Result representa el resultado de la descarga y procesamiento de una URL.
type Result struct {
	URL     string
	Title   string
	Content string
	Err     error
}

// Crawler maneja las descargas concurrentes de una lista de URLs.
type Crawler struct {
	workers    int
	httpClient *http.Client
}

// NewCrawler crea una nueva instancia de Crawler con un número determinado de trabajadores.
func NewCrawler(workers int) *Crawler {
	return &Crawler{
		workers: workers,
		httpClient: &http.Client{
			Timeout: 15 * time.Second, // Timeout por petición individual
		},
	}
}

// Crawl descarga de forma concurrente una lista de URLs y envía los resultados a un canal.
// Retorna un canal de lectura única para consumir los resultados a medida que se completan.
func (c *Crawler) Crawl(ctx context.Context, urls []string) <-chan Result {
	results := make(chan Result, len(urls))
	jobs := make(chan string, len(urls))

	// Encolar trabajos
	for _, url := range urls {
		jobs <- url
	}
	close(jobs)

	var wg sync.WaitGroup

	// Lanzar los Workers concurrentemente
	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					// Si el contexto global se cancela, abortamos el worker
					return
				case url, ok := <-jobs:
					if !ok {
						return // No quedan más trabajos en la cola
					}
					
					// Procesar la descarga de forma segura
					res := c.fetchAndParse(ctx, url)
					results <- res
				}
			}
		}(i)
	}

	// Cerrar el canal de resultados cuando todos los workers hayan terminado
	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// fetchAndParse realiza la llamada HTTP GET y parsea el cuerpo de la respuesta a Markdown.
func (c *Crawler) fetchAndParse(ctx context.Context, url string) Result {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Result{URL: url, Err: fmt.Errorf("error al crear request: %w", err)}
	}

	// Cabecera común de User-Agent para no ser bloqueados inmediatamente por las webs
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) DevBrainHarvester/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{URL: url, Err: fmt.Errorf("error de conexión HTTP: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{URL: url, Err: fmt.Errorf("estado HTTP incorrecto: %d %s", resp.StatusCode, resp.Status)}
	}

	title, content, err := parseHTML(resp.Body)
	if err != nil {
		return Result{URL: url, Err: fmt.Errorf("error parseando HTML: %w", err)}
	}

	if title == "" {
		title = url // Si no tiene tag title, usamos la URL como fallback
	}

	return Result{
		URL:     url,
		Title:   title,
		Content: content,
	}
}

// parseHTML recorre recursivamente el árbol DOM de HTML y genera una representación Markdown limpia.
// Filtra cabeceras flotantes, barras de navegación, footers, scripts y estilos CSS.
func parseHTML(r io.Reader) (string, string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", "", err
	}

	var titleBuilder strings.Builder
	var bodyBuilder strings.Builder

	var f func(*html.Node)
	f = func(n *html.Node) {
		// 1. Filtrado de ruido: ignorar etiquetas no informativas o scripts destructivos
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "nav" || tag == "footer" || tag == "header" || tag == "iframe" || tag == "aside" || tag == "noscript" {
				return // Ignoramos todo el subárbol
			}
		}

		// 2. Extraer el título del documento
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "title" {
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				titleBuilder.WriteString(strings.TrimSpace(n.FirstChild.Data))
			}
		}

		// 3. Procesar nodos de texto y transformarlos idiomáticamente a Markdown
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				parentTag := ""
				if n.Parent != nil && n.Parent.Type == html.ElementNode {
					parentTag = strings.ToLower(n.Parent.Data)
				}

				switch parentTag {
				case "h1":
					bodyBuilder.WriteString(fmt.Sprintf("\n\n# %s\n", text))
				case "h2":
					bodyBuilder.WriteString(fmt.Sprintf("\n\n## %s\n", text))
				case "h3":
					bodyBuilder.WriteString(fmt.Sprintf("\n\n### %s\n", text))
				case "p":
					bodyBuilder.WriteString(fmt.Sprintf("\n\n%s\n", text))
				case "code":
					// Si el abuelo es un <pre>, es un bloque de código completo
					isBlock := false
					if n.Parent.Parent != nil && strings.ToLower(n.Parent.Parent.Data) == "pre" {
						isBlock = true
					}
					if isBlock {
						bodyBuilder.WriteString(fmt.Sprintf("\n```go\n%s\n```\n", text))
					} else {
						bodyBuilder.WriteString(fmt.Sprintf(" `%s` ", text))
					}
				case "a":
					// Obtener el enlace href si existe
					href := ""
					for _, attr := range n.Parent.Attr {
						if strings.ToLower(attr.Key) == "href" {
							href = attr.Val
							break
						}
					}
					if href != "" && !strings.HasPrefix(href, "#") {
						bodyBuilder.WriteString(fmt.Sprintf(" [%s](%s) ", text, href))
					} else {
						bodyBuilder.WriteString(" " + text)
					}
				default:
					// Evitar acumulación de espacios vacíos en el parser
					bodyBuilder.WriteString(" " + text)
				}
			}
		}

		// Continuar recorriendo el árbol recursivamente para los hijos
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	// Limpieza final de saltos de línea repetitivos
	cleanBody := strings.TrimSpace(bodyBuilder.String())
	return strings.TrimSpace(titleBuilder.String()), cleanBody, nil
}
