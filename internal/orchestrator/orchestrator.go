package orchestrator

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// ScanProject recorre el directorio raíz del proyecto y devuelve un mapa estructural en Markdown.
func ScanProject(root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("error al resolver ruta absoluta: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 🏗️ Estructura del Proyecto: %s\n\n", filepath.Base(absRoot)))
	sb.WriteString("Esta es la topología física y arquitectura de directorios del proyecto, escaneada de forma automática.\n\n")

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Obtener la ruta relativa respecto a la raíz
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		// Particionar la ruta para contar la profundidad
		parts := strings.Split(relPath, string(filepath.Separator))
		depth := len(parts)

		// Comprobar si debemos ignorar este directorio/archivo
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".atl" {
				return filepath.SkipDir
			}
		}

		// Ignorar archivos sueltos comunes que no aportan valor estructural
		if !d.IsDir() {
			if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".exe") || name == "go.sum" {
				return nil
			}
		}

		// Generar la indentación visual en Markdown
		indent := strings.Repeat("  ", depth-1)
		if d.IsDir() {
			sb.WriteString(fmt.Sprintf("%s- 📁 **%s/**\n", indent, name))
		} else {
			sb.WriteString(fmt.Sprintf("%s- 📄 %s\n", indent, name))
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return sb.String(), nil
}
