package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"harvester/internal/crawler"
	"harvester/internal/orchestrator"
	"harvester/internal/storage"
)

// Server implementa el servidor MCP a través de Entrada/Salida estándar (Stdio).
type Server struct {
	store      *storage.Storage
	crawler    *crawler.Crawler
	writeMu    sync.Mutex // Protege la escritura concurrente en el stdout
	input      io.Reader
	output     io.Writer
}

// NewServer inicializa una nueva instancia del servidor MCP.
func NewServer(store *storage.Storage, input io.Reader, output io.Writer) *Server {
	return &Server{
		store:   store,
		crawler: crawler.NewCrawler(3), // 3 workers concurrentes para tareas del crawler
		input:   input,
		output:  output,
	}
}

// Start arranca el ciclo de lectura de Entrada Estándar y despacha peticiones concurrentemente.
func (s *Server) Start(ctx context.Context) error {
	scanner := bufio.NewScanner(s.input)

	// MCP requiere leer línea por línea donde cada línea es un objeto JSON-RPC completo
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, ErrCodeParseError, "Error al parsear petición JSON-RPC 2.0")
			continue
		}

		// Despachamos en una goroutine independiente para no bloquear el canal de lectura principal
		go s.handleRequest(ctx, req)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error leyendo stream MCP de entrada estándar: %w", err)
	}

	return nil
}

// handleRequest orquesta el enrutamiento de llamadas del cliente.
func (s *Server) handleRequest(ctx context.Context, req Request) {
	if req.JSONRPC != "2.0" {
		s.sendError(req.ID, ErrCodeInvalidRequest, "Petición inválida: Se requiere versión JSON-RPC 2.0")
		return
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleListTools(req)
	case "tools/call":
		s.handleCallTool(ctx, req)
	default:
		s.sendError(req.ID, ErrCodeMethodNotFound, fmt.Sprintf("Método MCP '%s' no implementado", req.Method))
	}
}

// handleInitialize procesa el saludo (handshake) inicial del cliente.
func (s *Server) handleInitialize(req Request) {
	res := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo: ServerInfo{
			Name:    "second-brain-mcp",
			Version: "1.0.0",
		},
	}
	res.Capabilities.Tools = struct{}{} // Informamos que implementamos Tools

	s.sendResponse(req.ID, res)
}

// handleListTools expone al LLM la lista de herramientas disponibles en nuestro cerebro.
func (s *Server) handleListTools(req Request) {
	tools := []Tool{
		{
			Name:        "remember",
			Description: "Guarda un hecho técnico, regla de código, snippet o preferencia del usuario en tu memoria local SQLite para autoaprendizaje continuo.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entity": {
						Type:        "string",
						Description: "El nombre de la entidad o tecnología asociada (ej: 'Go', 'FTS5', 'Docker').",
					},
					"content": {
						Type:        "string",
						Description: "La regla, hecho técnico, preferencia del usuario o bloque de código que se quiere memorizar.",
					},
					"source_url": {
						Type:        "string",
						Description: "Opcional. URL de origen o nombre de archivo de donde provino este conocimiento.",
					},
					"project": {
						Type:        "string",
						Description: "Opcional. El nombre del proyecto activo (carpeta del workspace) para restringir el ámbito del conocimiento, o 'global' si es un hecho general.",
					},
				},
				Required: []string{"entity", "content"},
			},
		},
		{
			Name:        "search_brain",
			Description: "Busca hechos, snippets y relaciones semánticas en la base de datos de memoria (Second Brain) mediante FTS5 y ordenamiento BM25.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query": {
						Type:        "string",
						Description: "El término de búsqueda semántico o palabra clave (ej: 'generics syntax', 'CGO Windows').",
					},
					"project": {
						Type:        "string",
						Description: "Opcional. El nombre del proyecto activo para buscar conocimientos de este proyecto más los globales transversales.",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "crawler_index",
			Description: "Carga en segundo plano la descarga, conversión a Markdown e indexación de un lote de URLs web en la base de datos.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"urls": {
						Type:        "array",
						Description: "Lote de direcciones HTTP a descargar de manera concurrente.",
						Items: &Items{
							Type: "string",
						},
					},
				},
				Required: []string{"urls"},
			},
		},
		{
			Name:        "index_project_structure",
			Description: "Escanea la estructura de archivos y directorios de un proyecto y la registra como conocimiento topológico local en tu cerebro SQLite.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"project_path": {
						Type:        "string",
						Description: "La ruta absoluta al directorio raíz del proyecto que se desea escanear.",
					},
					"project_name": {
						Type:        "string",
						Description: "El nombre identificativo del proyecto para aislar este conocimiento bajo su propio scope.",
					},
				},
				Required: []string{"project_path", "project_name"},
			},
		},
	}

	s.sendResponse(req.ID, ListToolsResult{Tools: tools})
}

// handleCallTool ejecuta la herramienta requerida por el LLM y responde el resultado de texto plano.
func (s *Server) handleCallTool(ctx context.Context, req Request) {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, ErrCodeInvalidParams, "Estructura de parámetros 'CallToolParams' inválida")
		return
	}

	var res CallToolResult

	switch params.Name {
	case "remember":
		res = s.toolRemember(params.Arguments)
	case "search_brain":
		res = s.toolSearchBrain(params.Arguments)
	case "crawler_index":
		res = s.toolCrawlerIndex(ctx, params.Arguments)
	case "index_project_structure":
		res = s.toolIndexProjectStructure(params.Arguments)
	default:
		s.sendError(req.ID, ErrCodeMethodNotFound, fmt.Sprintf("Herramienta '%s' no encontrada", params.Name))
		return
	}

	s.sendResponse(req.ID, res)
}

// === IMPLEMENTACIÓN DE LAS TOOLS (LÓGICA INTERNA) ===

type RememberArgs struct {
	Entity    string `json:"entity"`
	Content   string `json:"content"`
	SourceURL string `json:"source_url,omitempty"`
	Project   string `json:"project,omitempty"`
}

func (s *Server) toolRemember(argsJSON json.RawMessage) CallToolResult {
	var args RememberArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error parseando argumentos: %v", err)}},
		}
	}

	obsID, err := s.store.SaveObservation(args.Entity, args.Content, args.SourceURL, args.Project)
	if err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error al escribir memoria en SQLite: %v", err)}},
		}
	}

	return CallToolResult{
		Content: []ContentBlock{
			{
				Type: "text",
				Text: fmt.Sprintf("🧠 [MEMORIA PERSISTIDA] Guardé la observación #%d vinculada a '%s' con éxito en tu base de datos SQLite local.", obsID, args.Entity),
			},
		},
	}
}

type SearchBrainArgs struct {
	Query   string `json:"query"`
	Project string `json:"project,omitempty"`
}

func (s *Server) toolSearchBrain(argsJSON json.RawMessage) CallToolResult {
	var args SearchBrainArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error parseando argumentos: %v", err)}},
		}
	}

	results, err := s.store.SearchBrain(args.Query, args.Project, 5)
	if err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error consultando base de datos: %v", err)}},
		}
	}

	if len(results) == 0 {
		return CallToolResult{
			Content: []ContentBlock{
				{
					Type: "text",
					Text: "ℹ️  No se encontraron recuerdos, reglas ni documentación relevante para esa consulta en tu memoria local.",
				},
			},
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📖 Encontré %d hechos técnicos y reglas de interés en tu cerebro local (SQLite FTS5 BM25):\n\n", len(results)))
	for i, res := range results {
		sb.WriteString(fmt.Sprintf("[%d] 🏆 Relevancia BM25: %.4f | Entidad: %s (%s)\n", i+1, res.Rank, res.EntityName, res.EntityCategory))
		sb.WriteString(fmt.Sprintf("    📝 Detalle: %s\n", res.Content))
		
		// Inyectar relaciones semánticas
		if len(res.Relations) > 0 {
			sb.WriteString("    🔗 Conexiones Semánticas:\n")
			for _, rel := range res.Relations {
				sb.WriteString(fmt.Sprintf("      - %s\n", rel))
			}
		}

		if res.SourceURL != "" {
			sb.WriteString(fmt.Sprintf("    🔗 Origen: %s\n", res.SourceURL))
		}
		sb.WriteString("\n" + strings.Repeat("-", 60) + "\n")
	}

	return CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: sb.String()}},
	}
}

type IndexProjectStructureArgs struct {
	ProjectPath string `json:"project_path"`
	ProjectName string `json:"project_name"`
}

func (s *Server) toolIndexProjectStructure(argsJSON json.RawMessage) CallToolResult {
	var args IndexProjectStructureArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error parseando argumentos: %v", err)}},
		}
	}

	mdMap, err := orchestrator.ScanProject(args.ProjectPath)
	if err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error escaneando el directorio del proyecto: %v", err)}},
		}
	}

	// Guardar el mapa topológico en el cerebro SQLite bajo el project scope local
	obsID, err := s.store.SaveObservation("Project Topology", mdMap, args.ProjectPath, args.ProjectName)
	if err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error guardando el mapa topológico en SQLite: %v", err)}},
		}
	}

	return CallToolResult{
		Content: []ContentBlock{
			{
				Type: "text",
				Text: fmt.Sprintf("🏗️  [TOPOLOGÍA INDEXADA] Escaneé con éxito la estructura de directorios del proyecto y guardé el mapa topológico (observación #%d) bajo el scope local '%s' en tu cerebro local.", obsID, args.ProjectName),
			},
		},
	}
}

type CrawlerIndexArgs struct {
	URLs []string `json:"urls"`
}

func (s *Server) toolCrawlerIndex(ctx context.Context, argsJSON json.RawMessage) CallToolResult {
	var args CrawlerIndexArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error parseando argumentos: %v", err)}},
		}
	}

	// Lógica asíncrona: Corremos la descarga en background para responder de inmediato al cliente MCP!
	go func() {
		// Contexto aislado para el background worker
		bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		resultsChan := s.crawler.Crawl(bgCtx, args.URLs)
		for res := range resultsChan {
			if res.Err != nil {
				// CRÍTICO: Los errores en background NUNCA deben salir a stdout.
				// stdout está reservado exclusivamente para mensajes del protocolo MCP JSON-RPC.
				// Escribimos a stderr para que el host MCP los registre de forma segura sin corromper el canal de datos.
				fmt.Fprintf(os.Stderr, "⚠️ [CRAWLER BG ERROR] Error descargando %s: %v\n", res.URL, res.Err)
				continue
			}

			// Ingesta Fase 1
			_, err := s.store.SaveDocument(res.URL, res.Title, res.Content)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ [CRAWLER BG DB ERROR] Error al guardar documento: %v\n", err)
				continue
			}

			// Ingesta Fase 2 (Almacenar también en el cerebro autolimpiante)
			_, err = s.store.SaveObservation(res.Title, fmt.Sprintf("Documento indexado desde la web. Cuerpo: %s", res.Content), res.URL, "global")
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ [CRAWLER BG DB ERROR] Error al guardar observación semántica: %v\n", err)
			}
		}
	}()

	return CallToolResult{
		Content: []ContentBlock{
			{
				Type: "text",
				Text: fmt.Sprintf("🚀 El motor de descarga ha encolado %d URL(s) en segundo plano de manera concurrente. Los resultados se indexarán en caliente.", len(args.URLs)),
			},
		},
	}
}

// === AUXILIARES DE CONEXIÓN ===

func (s *Server) sendResponse(id interface{}, result interface{}) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	data, err := json.Marshal(resp)
	if err == nil {
		fmt.Fprintf(s.output, "%s\n", data)
	}
}

func (s *Server) sendError(id interface{}, code int, message string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   NewError(code, message),
	}

	data, err := json.Marshal(resp)
	if err == nil {
		fmt.Fprintf(s.output, "%s\n", data)
	}
}
