package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanProject(t *testing.T) {
	// 1. Crear un directorio temporal para simular un proyecto
	tempDir, err := os.MkdirTemp("", "test_project_scan")
	if err != nil {
		t.Fatalf("Error creando directorio temporal: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Crear subdirectorios de prueba
	dirs := []string{
		filepath.Join(tempDir, "cmd"),
		filepath.Join(tempDir, "internal"),
		filepath.Join(tempDir, "internal", "storage"),
		filepath.Join(tempDir, ".git"), // Debe ser ignorado
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("Error creando subdirectorio %s: %v", d, err)
		}
	}

	// Crear archivos de prueba
	files := map[string]string{
		filepath.Join(tempDir, "cmd", "main.go"):        "package main",
		filepath.Join(tempDir, "internal", "utils.go"): `package internal

type Requester interface {
	Request() error
}

type Config struct {
	URL string
}

func GetConfig() *Config {
	return &Config{}
}

func (c *Config) Validate() bool {
	return true
}
`,
		filepath.Join(tempDir, "go.mod"):                "module test_project",
		filepath.Join(tempDir, "harvester.db"):          "", // Debe ser ignorado
		filepath.Join(tempDir, ".git", "config"):        "", // Debe ser ignorado (está dentro de .git)
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Error creando archivo %s: %v", path, err)
		}
	}

	// 2. Ejecutar el escáner de proyecto
	md, err := ScanProject(tempDir)
	if err != nil {
		t.Fatalf("Error en ScanProject: %v", err)
	}

	// 3. Verificar exclusiones e inclusiones en el Markdown devuelto
	if !strings.Contains(md, "cmd") {
		t.Error("Se esperaba que el Markdown incluyera la carpeta 'cmd'")
	}
	if !strings.Contains(md, "main.go") {
		t.Error("Se esperaba que el Markdown incluyera el archivo 'main.go'")
	}
	if !strings.Contains(md, "utils.go") {
		t.Error("Se esperaba que el Markdown incluyera el archivo 'utils.go'")
	}
	if !strings.Contains(md, "go.mod") {
		t.Error("Se esperaba que el Markdown incluyera el archivo 'go.mod'")
	}

	// Verificar componentes parseados del AST
	if !strings.Contains(md, "Interfaces: `Requester`") {
		t.Errorf("Se esperaba que incluyera la interfaz 'Requester' en el AST. Obtenido:\n%s", md)
	}
	if !strings.Contains(md, "Structs: `Config`") {
		t.Errorf("Se esperaba que incluyera la estructura 'Config' en el AST. Obtenido:\n%s", md)
	}
	if !strings.Contains(md, "Funciones: `GetConfig`, `(*Config) Validate`") {
		t.Errorf("Se esperaba que incluyera las funciones 'GetConfig' y '(*Config) Validate'. Obtenido:\n%s", md)
	}

	// Validar que se hayan ignorado los archivos/carpetas correctos
	if strings.Contains(md, ".git") {
		t.Error("ScanProject falló en ignorar el directorio '.git'")
	}
	if strings.Contains(md, "harvester.db") {
		t.Error("ScanProject falló en ignorar la base de datos de extensión '.db'")
	}
}
