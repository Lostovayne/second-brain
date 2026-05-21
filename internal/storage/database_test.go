package storage

import (
	"os"
	"testing"
)

func TestStorageFTS5(t *testing.T) {
	tempDB := "test_harvester_f1.db"
	defer os.Remove(tempDB)

	s, err := NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage de test: %v", err)
	}
	defer s.Close()

	// 1. Probar la inserción/indexación de documentos
	url1 := "https://go.dev/doc/concurrency"
	title1 := "Go Concurrency Guide"
	content1 := "Goroutines and channels make concurrency easy in Go. Context handles cancellation."

	id1, err := s.SaveDocument(url1, title1, content1)
	if err != nil {
		t.Fatalf("Error al guardar primer documento: %v", err)
	}

	if id1 <= 0 {
		t.Errorf("ID del documento inválido: %d", id1)
	}

	// 2. Probar la búsqueda por palabra clave
	results, err := s.Search("concurrency", 5)
	if err != nil {
		t.Fatalf("Error al buscar 'concurrency': %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Se esperaba 1 resultado, se obtuvieron: %d", len(results))
	}
}

func TestStorageBrainSemantic(t *testing.T) {
	tempDB := "test_harvester_brain.db"
	defer os.Remove(tempDB)

	s, err := NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage de cerebro: %v", err)
	}
	defer s.Close()

	// 1. Guardar entidad explícita
	entID, err := s.SaveEntity("Golang", "language", "global")
	if err != nil {
		t.Fatalf("Error al guardar entidad: %v", err)
	}
	if entID <= 0 {
		t.Errorf("ID de entidad inválido: %d", entID)
	}

	// 2. Guardar observación (debe auto-crear la entidad si no existe)
	obsID1, err := s.SaveObservation("Golang", "Generics were introduced in Go 1.18.", "https://go.dev/blog/generics", "global")
	if err != nil {
		t.Fatalf("Error al guardar observación 1: %v", err)
	}
	if obsID1 <= 0 {
		t.Errorf("ID de observación inválido: %d", obsID1)
	}

	// Guardar otra observación (auto-creando entidad)
	obsID2, err := s.SaveObservation("Generics", "Generics allow writing code with type parameters.", "", "global")
	if err != nil {
		t.Fatalf("Error al guardar observación 2: %v", err)
	}
	if obsID2 <= 0 {
		t.Errorf("ID de observación 2 inválido: %d", obsID2)
	}

	// 3. Probar validación de duplicación (guardar la observación 1 exacta otra vez)
	obsID1Repeat, err := s.SaveObservation("Golang", "Generics were introduced in Go 1.18.", "https://go.dev/blog/generics", "global")
	if err != nil {
		t.Fatalf("Error al re-guardar observación: %v", err)
	}
	if obsID1Repeat != obsID1 {
		t.Errorf("Deduplicador falló: se esperaba ID %d, se obtuvo %d", obsID1, obsID1Repeat)
	}

	// 4. Crear relación semántica entre entidades
	err = s.LinkEntities("Golang", "Generics", "HAS_FEATURE", "global")
	if err != nil {
		t.Fatalf("Error al enlazar entidades: %v", err)
	}

	// 5. Realizar búsqueda semántica del cerebro usando BM25
	results, err := s.SearchBrain("Generics", "global", 5)
	if err != nil {
		t.Fatalf("Error en la búsqueda en el cerebro: %v", err)
	}

	// Debería traer 2 resultados (las dos observaciones que mencionan o pertenecen a Generics)
	if len(results) != 2 {
		t.Errorf("Se esperaban 2 resultados en el cerebro, se obtuvieron: %d", len(results))
	}

	// Verificar que la relación semántica de "Golang" fue inyectada al resultado correspondiente
	foundRelation := false
	for _, res := range results {
		if res.EntityName == "Golang" {
			if len(res.Relations) == 1 && res.Relations[0] == "HAS_FEATURE Generics" {
				foundRelation = true
			}
		}
	}

	if !foundRelation {
		t.Error("No se encontró la relación semántica 'HAS_FEATURE Generics' vinculada al resultado de 'Golang'")
	}
}

func TestStorageHierarchicalScoping(t *testing.T) {
	tempDB := "test_harvester_hierarchy.db"
	defer os.Remove(tempDB)

	s, err := NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage de jerarquía: %v", err)
	}
	defer s.Close()

	// 1. Guardar observación global
	_, err = s.SaveObservation("Go", "Go has native goroutines and channels.", "", "global")
	if err != nil {
		t.Fatalf("Error al guardar observación global: %v", err)
	}

	// 2. Guardar observación en Project A
	_, err = s.SaveObservation("Go", "Project A uses Go 1.25 for microservices.", "", "project-A")
	if err != nil {
		t.Fatalf("Error al guardar observación en project-A: %v", err)
	}

	// 3. Guardar observación en Project B
	_, err = s.SaveObservation("Go", "Project B uses Go 1.26 with structured logging.", "", "project-B")
	if err != nil {
		t.Fatalf("Error al guardar observación en project-B: %v", err)
	}

	// 4. Buscar desde la perspectiva de Project A
	resA, err := s.SearchBrain("Go", "project-A", 10)
	if err != nil {
		t.Fatalf("Error buscando en project-A: %v", err)
	}

	// Debería ver 2 observaciones: la global y la de project-A, pero NUNCA la de project-B
	if len(resA) != 2 {
		t.Errorf("Se esperaban 2 observaciones para project-A, se obtuvieron: %d", len(resA))
	}

	hasGlobal := false
	hasA := false
	hasB := false

	for _, res := range resA {
		if res.Content == "Go has native goroutines and channels." {
			hasGlobal = true
		}
		if res.Content == "Project A uses Go 1.25 for microservices." {
			hasA = true
		}
		if res.Content == "Project B uses Go 1.26 with structured logging." {
			hasB = true
		}
	}

	if !hasGlobal {
		t.Error("Búsqueda en project-A no heredó la observación global")
	}
	if !hasA {
		t.Error("Búsqueda en project-A no encontró su propia observación local")
	}
	if hasB {
		t.Error("Búsqueda en project-A filtró erróneamente e incluyó la observación de project-B")
	}

	// 5. Buscar desde la perspectiva de Project B
	resB, err := s.SearchBrain("Go", "project-B", 10)
	if err != nil {
		t.Fatalf("Error buscando en project-B: %v", err)
	}

	if len(resB) != 2 {
		t.Errorf("Se esperaban 2 observaciones para project-B, se obtuvieron: %d", len(resB))
	}

	hasGlobal = false
	hasA = false
	hasB = false

	for _, res := range resB {
		if res.Content == "Go has native goroutines and channels." {
			hasGlobal = true
		}
		if res.Content == "Project A uses Go 1.25 for microservices." {
			hasA = true
		}
		if res.Content == "Project B uses Go 1.26 with structured logging." {
			hasB = true
		}
	}

	if !hasGlobal {
		t.Error("Búsqueda en project-B no heredó la observación global")
	}
	if !hasB {
		t.Error("Búsqueda en project-B no encontró su propia observación local")
	}
	if hasA {
		t.Error("Búsqueda en project-B filtró erróneamente e incluyó la observación de project-A")
	}

	// 6. Buscar desde la perspectiva de Project C (vacío)
	resC, err := s.SearchBrain("Go", "project-C", 10)
	if err != nil {
		t.Fatalf("Error buscando en project-C: %v", err)
	}

	if len(resC) != 1 {
		t.Errorf("Se esperaba 1 observación para project-C (sólo la global), se obtuvieron: %d", len(resC))
	}

	if resC[0].Content != "Go has native goroutines and channels." {
		t.Errorf("Contenido incorrecto para project-C: %s", resC[0].Content)
	}
}
