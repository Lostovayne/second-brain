package brain

import (
	"os"
	"testing"

	"harvester/internal/storage"
)

func TestJanitor_Deduplication(t *testing.T) {
	tempDB := "test_janitor_dedup.db"
	defer os.Remove(tempDB)

	store, err := storage.NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage: %v", err)
	}
	defer store.Close()

	// 1. Insertar dos observaciones redundantes para la entidad "Docker"
	entity := "Docker"
	obs1 := "Docker enables containerized applications with extremely simple configurations."
	obs2 := "Docker enables containerized applications with simple configurations." // Más corto, duplicado cercano

	id1, err := store.SaveObservation(entity, obs1, "https://docker.com")
	if err != nil {
		t.Fatalf("Error al guardar obs1: %v", err)
	}
	id2, err := store.SaveObservation(entity, obs2, "https://docker.com")
	if err != nil {
		t.Fatalf("Error al guardar obs2: %v", err)
	}

	// 2. Correr el proceso de deduplicación del almacenamiento
	deleted, err := store.Deduplicate()
	if err != nil {
		t.Fatalf("Error al ejecutar deduplicación: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Se esperaba que se eliminara 1 observación, se eliminaron: %d", deleted)
	}

	// 3. Buscar en el cerebro y verificar que se conservó la observación más larga (obs1)
	results, err := store.SearchBrain("Docker", 5)
	if err != nil {
		t.Fatalf("Error en búsqueda del cerebro: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Se esperaba 1 resultado sobreviviente, se obtuvieron: %d", len(results))
	}

	survivor := results[0]
	if survivor.Content != obs1 {
		t.Errorf("Deduplicación falló en conservar la observación más larga.\nEsperado: %s\nObtenido: %s", obs1, survivor.Content)
	}

	if survivor.ObservationID != id1 {
		t.Errorf("Se eliminó el ID incorrecto. Se esperaba conservar el ID %d (obs1), pero sobrevivió el ID %d", id1, survivor.ObservationID)
	}

	// Comprobar que el ID eliminado ya no está accesible en FTS5
	ftsResults, err := store.SearchBrain(obs2, 5)
	if err != nil {
		t.Fatalf("Error buscando en FTS: %v", err)
	}
	for _, res := range ftsResults {
		if res.ObservationID == id2 {
			t.Errorf("La observación duplicada eliminada (ID %d) sigue presente en el índice FTS5", id2)
		}
	}
}

func TestJanitor_AutoLink(t *testing.T) {
	tempDB := "test_janitor_autolink.db"
	defer os.Remove(tempDB)

	store, err := storage.NewStorage(tempDB)
	if err != nil {
		t.Fatalf("Error al inicializar Storage: %v", err)
	}
	defer store.Close()

	// 1. Guardar observación con un verbo gatillo ("depends on")
	// La entidad origen es "React" y debe auto-vincular a "JavaScript" con tipo de relación "DEPENDS_ON".
	sourceEntity := "React"
	content := "React is a UI component library that depends on JavaScript."
	
	_, err = store.SaveObservation(sourceEntity, content, "")
	if err != nil {
		t.Fatalf("Error al guardar observación: %v", err)
	}

	// 2. Correr el auto-link semántico
	linked, err := store.AutoLink()
	if err != nil {
		t.Fatalf("Error al ejecutar auto-link: %v", err)
	}

	if linked <= 0 {
		t.Errorf("Se esperaba que se crearan enlaces semánticos automáticos, se crearon: %d", linked)
	}

	// 3. Buscar "React" en el cerebro y verificar que tiene la conexión inyectada
	results, err := store.SearchBrain("React", 5)
	if err != nil {
		t.Fatalf("Error al buscar React: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("No se encontraron resultados para React")
	}

	var foundRelation bool
	for _, res := range results {
		if res.EntityName == "React" {
			for _, rel := range res.Relations {
				if rel == "DEPENDS_ON JavaScript" {
					foundRelation = true
				}
			}
		}
	}

	if !foundRelation {
		t.Errorf("El autoaprendizaje falló en detectar y enlazar 'React DEPENDS_ON JavaScript'. Relaciones obtenidas: %v", results[0].Relations)
	}
}
