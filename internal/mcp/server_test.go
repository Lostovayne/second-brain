package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harvester/internal/storage"
)

func TestMCPServer_HandshakeAndTools(t *testing.T) {
	tempDB := "test_mcp_server.db"
	defer os.Remove(tempDB)

	store, err := storage.NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage: %v", err)
	}
	defer store.Close()

	// 1. Crear buffers mock para stdin/stdout
	var stdin bytes.Buffer
	var stdout bytes.Buffer

	// Petición 1: initialize handshake
	req1 := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	data1, _ := json.Marshal(req1)
	stdin.Write(data1)
	stdin.WriteByte('\n')

	// Petición 2: list tools
	req2 := Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	data2, _ := json.Marshal(req2)
	stdin.Write(data2)
	stdin.WriteByte('\n')

	// 2. Levantar el servidor MCP con un contexto cancelable
	server := NewServer(store, &stdin, &stdout)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Arrancamos el server y se detendrá cuando la entrada stdin se acabe (EOF)
	err = server.Start(ctx)
	if err != nil {
		t.Fatalf("Error al arrancar el servidor MCP: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 3. Parsear y verificar las respuestas impresas en stdout line-by-line
	outputLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(outputLines) < 2 {
		t.Fatalf("Se esperaban al menos 2 líneas de respuesta en stdout, se obtuvieron: %d. Contenido: %q", len(outputLines), stdout.String())
	}

	// Mapear respuestas por su ID JSON-RPC para evitar fallos de ordenamiento no-determinista
	responses := make(map[float64]Response)
	for _, line := range outputLines {
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("Error al deserializar respuesta: %v", err)
		}
		if idNum, ok := resp.ID.(float64); ok {
			responses[idNum] = resp
		}
	}

	// Verificar respuesta 1 (initialize)
	resp1, found := responses[1]
	if !found {
		t.Fatalf("No se encontró la respuesta para el ID 1. Respuestas obtenidas: %v", responses)
	}
	
	var initResult InitializeResult
	initResultBytes, _ := json.Marshal(resp1.Result)
	json.Unmarshal(initResultBytes, &initResult)
	if initResult.ServerInfo.Name != "second-brain-mcp" {
		t.Errorf("Nombre de servidor incorrecto: %s", initResult.ServerInfo.Name)
	}

	// Verificar respuesta 2 (tools/list)
	resp2, found := responses[2]
	if !found {
		t.Fatalf("No se encontró la respuesta para el ID 2. Respuestas obtenidas: %v", responses)
	}

	var listResult ListToolsResult
	listResultBytes, _ := json.Marshal(resp2.Result)
	json.Unmarshal(listResultBytes, &listResult)

	if len(listResult.Tools) != 4 {
		t.Errorf("Se esperaban 4 herramientas expuestas, se obtuvieron: %d", len(listResult.Tools))
	}

	// Verificar nombres de las herramientas
	expectedTools := map[string]bool{
		"remember":                true,
		"search_brain":            true,
		"crawler_index":           true,
		"index_project_structure": true,
	}
	for _, tool := range listResult.Tools {
		if !expectedTools[tool.Name] {
			t.Errorf("Herramienta inesperada expuesta por el servidor: %s", tool.Name)
		}
	}
}

func TestMCPServer_ToolExecution(t *testing.T) {
	tempDB := "test_mcp_tools.db"
	defer os.Remove(tempDB)

	store, err := storage.NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage: %v", err)
	}
	defer store.Close()

	// Pre-poblar la base de datos de antemano.
	// Esto asegura que la búsqueda FTS5 del "search_brain" siempre sea exitosa
	// sin importar si la goroutine de remember corre antes o después del search.
	_, err = store.SaveObservation("Golang", "Go uses channels for goroutine communication.", "https://go.dev", "global")
	if err != nil {
		t.Fatalf("Error al pre-poblar base de datos: %v", err)
	}

	var stdin bytes.Buffer
	var stdout bytes.Buffer

	// Petición 1: Llamar a la herramienta "remember" (para guardar una nueva observación)
	rememberArgs := RememberArgs{
		Entity:    "Golang",
		Content:   "Go uses select to multiplex channel operations.",
		SourceURL: "https://go.dev",
	}
	argsBytes, _ := json.Marshal(rememberArgs)
	
	callParams := CallToolParams{
		Name:      "remember",
		Arguments: argsBytes,
	}
	paramsBytes, _ := json.Marshal(callParams)

	req := Request{
		JSONRPC: "2.0",
		ID:      10,
		Method:  "tools/call",
		Params:  paramsBytes,
	}
	reqBytes, _ := json.Marshal(req)
	stdin.Write(reqBytes)
	stdin.WriteByte('\n')

	// Petición 2: Llamar a la herramienta "search_brain" (buscando lo pre-poblado)
	searchArgs := SearchBrainArgs{
		Query: "channels",
	}
	searchArgsBytes, _ := json.Marshal(searchArgs)
	
	searchParams := CallToolParams{
		Name:      "search_brain",
		Arguments: searchArgsBytes,
	}
	searchParamsBytes, _ := json.Marshal(searchParams)

	req2 := Request{
		JSONRPC: "2.0",
		ID:      11,
		Method:  "tools/call",
		Params:  searchParamsBytes,
	}
	req2Bytes, _ := json.Marshal(req2)
	stdin.Write(req2Bytes)
	stdin.WriteByte('\n')

	server := NewServer(store, &stdin, &stdout)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = server.Start(ctx)
	if err != nil {
		t.Fatalf("Error al arrancar el servidor MCP: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	outputLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(outputLines) < 2 {
		t.Fatalf("Se esperaban al menos 2 líneas de respuesta en stdout, se obtuvieron: %d. Contenido: %q", len(outputLines), stdout.String())
	}

	responses := make(map[float64]Response)
	for _, line := range outputLines {
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("Error al deserializar respuesta: %v", err)
		}
		if idNum, ok := resp.ID.(float64); ok {
			responses[idNum] = resp
		}
	}

	// Verificar respuesta de remember (ID 10)
	resp1, found := responses[10]
	if !found {
		t.Fatalf("No se encontró la respuesta para el ID 10. Respuestas: %v", responses)
	}

	var callResult1 CallToolResult
	resBytes1, _ := json.Marshal(resp1.Result)
	json.Unmarshal(resBytes1, &callResult1)
	if callResult1.IsError {
		t.Errorf("Error durante la ejecución del remember: %v", callResult1.Content[0].Text)
	}
	if !strings.Contains(callResult1.Content[0].Text, "MEMORIA PERSISTIDA") {
		t.Errorf("Respuesta de remember inesperada: %s", callResult1.Content[0].Text)
	}

	// Verificar respuesta de search_brain (ID 11)
	resp2, found := responses[11]
	if !found {
		t.Fatalf("No se encontró la respuesta para el ID 11. Respuestas: %v", responses)
	}

	var callResult2 CallToolResult
	resBytes2, _ := json.Marshal(resp2.Result)
	json.Unmarshal(resBytes2, &callResult2)
	if callResult2.IsError {
		t.Errorf("Error durante la ejecución de search_brain: %v", callResult2.Content[0].Text)
	}
	if !strings.Contains(callResult2.Content[0].Text, "Go uses channels") {
		t.Errorf("Resultado de búsqueda no contenía el texto indexado. Respuesta: %s", callResult2.Content[0].Text)
	}
}

func TestMCPServer_IndexProjectStructure(t *testing.T) {
	tempDB := "test_mcp_orchestrator.db"
	defer os.Remove(tempDB)

	store, err := storage.NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage: %v", err)
	}
	defer store.Close()

	// Crear proyecto mock en directorio temporal
	tempProjDir, err := os.MkdirTemp("", "mcp_test_proj")
	if err != nil {
		t.Fatalf("Error creando temp proj dir: %v", err)
	}
	defer os.RemoveAll(tempProjDir)

	err = os.MkdirAll(filepath.Join(tempProjDir, "cmd"), 0755)
	if err != nil {
		t.Fatalf("Error creando cmd dir: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempProjDir, "cmd", "main.go"), []byte("package main"), 0644)
	if err != nil {
		t.Fatalf("Error escribiendo main.go: %v", err)
	}

	var stdin bytes.Buffer
	var stdout bytes.Buffer

	// Petición: index_project_structure
	indexArgs := IndexProjectStructureArgs{
		ProjectPath: tempProjDir,
		ProjectName: "demo-project",
	}
	argsBytes, _ := json.Marshal(indexArgs)

	callParams := CallToolParams{
		Name:      "index_project_structure",
		Arguments: argsBytes,
	}
	paramsBytes, _ := json.Marshal(callParams)

	req := Request{
		JSONRPC: "2.0",
		ID:      20,
		Method:  "tools/call",
		Params:  paramsBytes,
	}
	reqBytes, _ := json.Marshal(req)
	stdin.Write(reqBytes)
	stdin.WriteByte('\n')

	server := NewServer(store, &stdin, &stdout)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = server.Start(ctx)
	if err != nil {
		t.Fatalf("Error al arrancar el servidor MCP: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	var resp Response
	err = json.Unmarshal(stdout.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Error al unmarshal de respuesta: %v. Contenido: %q", err, stdout.String())
	}

	var callResult CallToolResult
	resBytes, _ := json.Marshal(resp.Result)
	json.Unmarshal(resBytes, &callResult)

	if callResult.IsError {
		t.Errorf("La ejecución del tool falló con error: %v", callResult.Content[0].Text)
	}

	if !strings.Contains(callResult.Content[0].Text, "TOPOLOGÍA INDEXADA") {
		t.Errorf("Respuesta inesperada: %s", callResult.Content[0].Text)
	}

	// Verificar directamente en SQLite que la topología fue registrada bajo el proyecto
	results, err := store.SearchBrain("Estructura del Proyecto", "demo-project", 5)
	if err != nil {
		t.Fatalf("Error consultando la topología guardada en SQLite: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Se esperaba encontrar 1 registro de topología, se encontraron: %d", len(results))
	}

	if !strings.Contains(results[0].Content, "cmd") || !strings.Contains(results[0].Content, "main.go") {
		t.Errorf("La topología no contenía la estructura correcta. Obtenido: %s", results[0].Content)
	}
}

