package mcp

import (
	"encoding/json"
)

// Constante para la versión del protocolo MCP oficial
const ProtocolVersion = "2024-11-05"

// Request representa una petición JSON-RPC 2.0 estándar recibida del cliente.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"` // Puede ser string, int o float64
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response representa una respuesta JSON-RPC 2.0 estándar enviada al cliente.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Notification representa un mensaje unidireccional JSON-RPC 2.0 sin respuesta requerida.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Error encapsula los códigos y mensajes de error JSON-RPC 2.0 estándar.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Códigos de error estándar JSON-RPC 2.0
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// NewError crea un objeto de error estándar.
func NewError(code int, msg string) *Error {
	return &Error{
		Code:    code,
		Message: msg,
	}
}

// === ESTRUCTURAS ESPECÍFICAS DE MCP ===

// InitializeParams representa los parámetros recibidos durante el saludo inicial (initialize).
type InitializeParams struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ClientInfo      ClientInfo      `json:"clientInfo"`
}

// ClientInfo contiene metadatos del cliente que se conecta al servidor.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult es la respuesta al handshake 'initialize'. Expone lo que el servidor puede hacer.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ServerCapabilities expone las APIs que nuestro servidor implementa (en nuestro caso, Tools).
type ServerCapabilities struct {
	Tools struct{} `json:"tools"`
}

// ServerInfo contiene metadatos del servidor.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool representa la definición de una herramienta MCP expuesta al LLM.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema define la estructura JSON Schema para los argumentos que requiere una Tool.
type InputSchema struct {
	Type       string              `json:"type"` // Siempre 'object'
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property define el tipo y descripción de un argumento individual de la Tool.
type Property struct {
	Type        string   `json:"type"` // 'string', 'number', 'boolean', 'array'
	Description string   `json:"description"`
	Items       *Items   `json:"items,omitempty"` // Requerido si es un array
}

// Items define los tipos de elementos de una propiedad de tipo array.
type Items struct {
	Type string `json:"type"`
}

// ListToolsResult es la respuesta a la petición 'tools/list'.
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolParams contiene los argumentos para invocar una Tool.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// CallToolResult es el formato estándar para responder tras la ejecución de una Tool.
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// ContentBlock representa un bloque individual de respuesta de texto plano que leerá el LLM.
type ContentBlock struct {
	Type string `json:"type"` // Siempre 'text' en nuestro servidor simple
	Text string `json:"text"`
}
