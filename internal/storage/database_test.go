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
	entID, err := s.SaveEntity("Golang", "language")
	if err != nil {
		t.Fatalf("Error al guardar entidad: %v", err)
	}
	if entID <= 0 {
		t.Errorf("ID de entidad inválido: %d", entID)
	}

	// 2. Guardar observación (debe auto-crear la entidad si no existe)
	obsID1, err := s.SaveObservation("Golang", "Generics were introduced in Go 1.18.", "https://go.dev/blog/generics")
	if err != nil {
		t.Fatalf("Error al guardar observación 1: %v", err)
	}
	if obsID1 <= 0 {
		t.Errorf("ID de observación inválido: %d", obsID1)
	}

	// Guardar otra observación (auto-creando entidad)
	obsID2, err := s.SaveObservation("Generics", "Generics allow writing code with type parameters.", "")
	if err != nil {
		t.Fatalf("Error al guardar observación 2: %v", err)
	}
	if obsID2 <= 0 {
		t.Errorf("ID de observación 2 inválido: %d", obsID2)
	}

	// 3. Probar validación de duplicación (guardar la observación 1 exacta otra vez)
	obsID1Repeat, err := s.SaveObservation("Golang", "Generics were introduced in Go 1.18.", "https://go.dev/blog/generics")
	if err != nil {
		t.Fatalf("Error al re-guardar observación: %v", err)
	}
	if obsID1Repeat != obsID1 {
		t.Errorf("Deduplicador falló: se esperaba ID %d, se obtuvo %d", obsID1, obsID1Repeat)
	}

	// 4. Crear relación semántica entre entidades
	err = s.LinkEntities("Golang", "Generics", "HAS_FEATURE")
	if err != nil {
		t.Fatalf("Error al enlazar entidades: %v", err)
	}

	// 5. Realizar búsqueda semántica del cerebro usando BM25
	results, err := s.SearchBrain("Generics", 5)
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
