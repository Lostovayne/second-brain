package orchestrator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// parseGoFileAST analiza un archivo de Go y extrae tipos (structs/interfaces) y funciones públicas.
func parseGoFileAST(path string) (structs []string, interfaces []string, funcs []string, err error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if !ast.IsExported(typeSpec.Name.Name) {
						continue
					}
					switch typeSpec.Type.(type) {
					case *ast.StructType:
						structs = append(structs, typeSpec.Name.Name)
					case *ast.InterfaceType:
						interfaces = append(interfaces, typeSpec.Name.Name)
					}
				}
			}
		case *ast.FuncDecl:
			if !ast.IsExported(d.Name.Name) {
				continue
			}
			var recv string
			if d.Recv != nil && len(d.Recv.List) > 0 {
				t := d.Recv.List[0].Type
				switch expr := t.(type) {
				case *ast.Ident:
					recv = expr.Name
				case *ast.StarExpr:
					if ident, ok := expr.X.(*ast.Ident); ok {
						recv = "*" + ident.Name
					}
				}
			}
			if recv != "" {
				funcs = append(funcs, fmt.Sprintf("(%s) %s", recv, d.Name.Name))
			} else {
				funcs = append(funcs, d.Name.Name)
			}
		}
	}
	return structs, interfaces, funcs, nil
}

// ScanProject recorre el directorio raíz del proyecto y devuelve un mapa estructural en Markdown con detalles del AST.
func ScanProject(root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("error al resolver ruta absoluta: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Estructura del Proyecto: %s\n\n", filepath.Base(absRoot)))
	sb.WriteString("Esta es la topología física y arquitectura de directorios del proyecto, escaneada de forma automática.\n\n")

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		depth := len(parts)

		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".atl" ||
				name == "dist" || name == "build" || name == "bin" || name == ".vscode" || name == ".idea" {
				return filepath.SkipDir
			}
		}

		if !d.IsDir() {
			if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".exe") || name == "go.sum" {
				return nil
			}
		}

		indent := strings.Repeat("  ", depth-1)
		if d.IsDir() {
			sb.WriteString(fmt.Sprintf("%s- [DIR] **%s/**\n", indent, name))
		} else {
			sb.WriteString(fmt.Sprintf("%s- [FILE] %s", indent, name))
			// Si es un archivo de Go, realizamos el parseo AST liviano
			if strings.HasSuffix(name, ".go") {
				structs, interfaces, funcs, err := parseGoFileAST(path)
				if err == nil {
					var details []string
					if len(structs) > 0 {
						details = append(details, fmt.Sprintf("Structs: `%s`", strings.Join(structs, "`, `")))
					}
					if len(interfaces) > 0 {
						details = append(details, fmt.Sprintf("Interfaces: `%s`", strings.Join(interfaces, "`, `")))
					}
					if len(funcs) > 0 {
						details = append(details, fmt.Sprintf("Funciones: `%s`", strings.Join(funcs, "`, `")))
					}

					if len(details) > 0 {
						sb.WriteString(fmt.Sprintf("\n%s  * %s *", indent, strings.Join(details, " | ")))
					}
				}
			}
			sb.WriteString("\n")
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return sb.String(), nil
}
