package storage

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// === ESTRUCTURAS DE LA FASE 1 ===

// Document representa la metadata y contenido de un archivo/web indexado.
type Document struct {
	ID        int64
	URL       string
	Title     string
	Content   string
	CreatedAt string
}

// SearchResult representa un resultado de búsqueda ordenado por BM25 con su fragmento de coincidencia.
type SearchResult struct {
	DocID   int64   `json:"doc_id"`
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Rank    float64 `json:"rank"`
}

// === ESTRUCTURAS DE LA FASE 2 ("Second Brain") ===

// Entity representa un concepto o preferencia única en el cerebro.
type Entity struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"` // 'library', 'language', 'user_preference', 'rule', 'concept'
	UpdatedAt string `json:"updated_at"`
}

// Observation representa un hecho o regla técnica asociada a una entidad.
type Observation struct {
	ID        int64  `json:"id"`
	EntityID  int64  `json:"entity_id"`
	Content   string `json:"content"`
	SourceURL string `json:"source_url,omitempty"`
	CreatedAt string `json:"created_at"`
}

// BrainSearchResult representa un resultado enriquecido para búsquedas en el segundo cerebro.
type BrainSearchResult struct {
	ObservationID  int64    `json:"observation_id"`
	EntityName     string   `json:"entity_name"`
	EntityCategory string   `json:"entity_category"`
	Content        string   `json:"content"`
	SourceURL      string   `json:"source_url,omitempty"`
	Snippet        string   `json:"snippet"`
	Rank           float64  `json:"rank"`
	Relations      []string `json:"relations,omitempty"` // Relaciones semánticas conectadas
}

// === CONTROLLER & CONEXIÓN ===

// Storage maneja la conexión a SQLite y las operaciones FTS5.
type Storage struct {
	db *sql.DB
}

// NewStorage abre una conexión a la base de datos SQLite y ejecuta las migraciones iniciales.
func NewStorage(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error al abrir la base de datos: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("error de ping a la base de datos: %w", err)
	}

	s := &Storage{db: db}
	if err := s.bootstrap(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// bootstrap crea las tablas de la Fase 1 y la Fase 2 si no existen.
func (s *Storage) bootstrap() error {
	queries := []string{
		// --- TABLAS FASE 1 ---
		`CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT UNIQUE NOT NULL,
			title TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
			doc_id UNINDEXED,
			title,
			content,
			tokenize="porter unicode61"
		);`,

		// --- TABLAS FASE 2 ("Second Brain" Semántico) ---
		// Entidades
		`CREATE TABLE IF NOT EXISTS entities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			category TEXT NOT NULL, -- 'library', 'language', 'user_preference', 'rule', 'concept'
			project TEXT DEFAULT 'global',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		// Observaciones
		`CREATE TABLE IF NOT EXISTS observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			source_url TEXT,
			project TEXT DEFAULT 'global',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(entity_id) REFERENCES entities(id) ON DELETE CASCADE
		);`,

		// Relaciones semánticas direccionales
		`CREATE TABLE IF NOT EXISTS relations (
			source_id INTEGER NOT NULL,
			target_id INTEGER NOT NULL,
			relation_type TEXT NOT NULL, -- 'DEPRECATED_BY', 'COMPATIBLE_WITH', 'USES', 'DEPENDS_ON'
			project TEXT DEFAULT 'global',
			PRIMARY KEY (source_id, target_id, relation_type, project),
			FOREIGN KEY(source_id) REFERENCES entities(id) ON DELETE CASCADE,
			FOREIGN KEY(target_id) REFERENCES entities(id) ON DELETE CASCADE
		);`,

		// Tabla virtual FTS5 para indexar las observaciones conectadas
		`CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
			observation_id UNINDEXED,
			entity_name,
			content,
			tokenize="porter unicode61"
		);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("error ejecutando migración: %w", err)
		}
	}

	// --- MIGRACIÓN EN CALIENTE / ZERO-DOWNTIME ---
	// Dado que el usuario ya tiene 'harvester.db' creado en fases anteriores, 
	// ejecutamos sentencias ALTER TABLE silenciosas para agregar la columna 'project'
	// en caso de que las tablas ya existieran previamente sin ella.
	_, _ = s.db.Exec("ALTER TABLE entities ADD COLUMN project TEXT DEFAULT 'global'")
	_, _ = s.db.Exec("ALTER TABLE observations ADD COLUMN project TEXT DEFAULT 'global'")
	_, _ = s.db.Exec("ALTER TABLE relations ADD COLUMN project TEXT DEFAULT 'global'")

	return nil
}

// === MÉTODOS DE LA FASE 1 (Documentos y Crawler) ===

func (s *Storage) SaveDocument(url, title, content string) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("error al iniciar transacción: %w", err)
	}
	defer tx.Rollback()

	var docID int64
	err = tx.QueryRow("SELECT id FROM documents WHERE url = ?", url).Scan(&docID)
	if err == sql.ErrNoRows {
		res, err := tx.Exec("INSERT INTO documents (url, title) VALUES (?, ?)", url, title)
		if err != nil {
			return 0, fmt.Errorf("error al insertar documento: %w", err)
		}
		docID, err = res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("error al obtener ID insertado: %w", err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("error al consultar documento existente: %w", err)
	} else {
		_, err = tx.Exec("UPDATE documents SET title = ?, created_at = CURRENT_TIMESTAMP WHERE id = ?", title, docID)
		if err != nil {
			return 0, fmt.Errorf("error al actualizar documento: %w", err)
		}
		_, err = tx.Exec("DELETE FROM documents_fts WHERE doc_id = ?", docID)
		if err != nil {
			return 0, fmt.Errorf("error al limpiar índice FTS5 anterior: %w", err)
		}
	}

	_, err = tx.Exec("INSERT INTO documents_fts (doc_id, title, content) VALUES (?, ?, ?)", docID, title, content)
	if err != nil {
		return 0, fmt.Errorf("error al indexar texto en FTS5: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("error al comprometer transacción: %w", err)
	}

	return docID, nil
}

func (s *Storage) Search(query string, limit int) ([]SearchResult, error) {
	sqlQuery := `
		SELECT 
			d.id, 
			d.url, 
			d.title, 
			snippet(documents_fts, 2, '***', '***', '...', 32) as snippet, 
			rank
		FROM documents_fts
		JOIN documents d ON d.id = documents_fts.doc_id
		WHERE documents_fts MATCH ?
		ORDER BY rank ASC
		LIMIT ?;
	`

	cleanedQuery := sanitizeQuery(query)
	rows, err := s.db.Query(sqlQuery, cleanedQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("error en la búsqueda FTS5 con query '%s' (sanitizado: '%s'): %w", query, cleanedQuery, err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var res SearchResult
		if err := rows.Scan(&res.DocID, &res.URL, &res.Title, &res.Snippet, &res.Rank); err != nil {
			return nil, fmt.Errorf("error al escanear resultado de búsqueda: %w", err)
		}
		results = append(results, res)
	}

	return results, nil
}

// === MÉTODOS DE LA FASE 2 ("Second Brain" Semántico) ===

// SaveEntity guarda o recupera una entidad según su nombre único (case-insensitive).
// SaveEntity guarda o recupera una entidad según su nombre único (case-insensitive).
func (s *Storage) SaveEntity(name, category, project string) (int64, error) {
	name = strings.TrimSpace(name)
	category = strings.TrimSpace(category)
	project = strings.TrimSpace(project)
	if name == "" {
		return 0, fmt.Errorf("el nombre de la entidad no puede estar vacío")
	}
	if category == "" {
		category = "concept"
	}
	if project == "" {
		project = "global"
	}

	var id int64
	// SQLite por defecto hace comparaciones case-insensitive si definimos campos como TEXT en queries simples,
	// pero para estar 100% seguros buscamos usando un matching exacto.
	err := s.db.QueryRow("SELECT id FROM entities WHERE LOWER(name) = LOWER(?)", name).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec("INSERT INTO entities (name, category, project) VALUES (?, ?, ?)", name, category, project)
		if err != nil {
			return 0, fmt.Errorf("error al insertar entidad: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("error al obtener ID de entidad: %w", err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("error al buscar entidad: %w", err)
	}

	return id, nil
}

// SaveObservation asocia un hecho u observación técnica a una entidad e indexa en FTS5.
func (s *Storage) SaveObservation(entityName, content, sourceURL, project string) (int64, error) {
	entityName = strings.TrimSpace(entityName)
	content = strings.TrimSpace(content)
	sourceURL = strings.TrimSpace(sourceURL)
	project = strings.TrimSpace(project)

	if entityName == "" || content == "" {
		return 0, fmt.Errorf("el nombre de entidad y el contenido de observación no pueden estar vacíos")
	}
	if project == "" {
		project = "global"
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("error al iniciar transacción de observación: %w", err)
	}
	defer tx.Rollback()

	// 1. Obtener o auto-crear la Entidad
	var entityID int64
	err = tx.QueryRow("SELECT id FROM entities WHERE LOWER(name) = LOWER(?)", entityName).Scan(&entityID)
	if err == sql.ErrNoRows {
		res, err := tx.Exec("INSERT INTO entities (name, category, project) VALUES (?, 'concept', ?)", entityName, project)
		if err != nil {
			return 0, fmt.Errorf("error al auto-crear entidad: %w", err)
		}
		entityID, err = res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("error al obtener ID de entidad auto-creada: %w", err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("error al buscar entidad: %w", err)
	}

	// 2. Comprobar si la observación exacta ya existe para evitar meter basura duplicada en este proyecto
	var obsID int64
	err = tx.QueryRow("SELECT id FROM observations WHERE entity_id = ? AND content = ? AND project = ?", entityID, content, project).Scan(&obsID)
	if err == nil {
		// Duplicado exacto detectado: retornamos el ID existente pacíficamente
		return obsID, nil
	} else if err != sql.ErrNoRows {
		return 0, fmt.Errorf("error al validar existencia de la observación: %w", err)
	}

	// 3. Insertar observación
	res, err := tx.Exec("INSERT INTO observations (entity_id, content, source_url, project) VALUES (?, ?, ?, ?)", entityID, content, sourceURL, project)
	if err != nil {
		return 0, fmt.Errorf("error al insertar observación: %w", err)
	}
	obsID, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("error al obtener ID de observación: %w", err)
	}

	// 4. Indexar atómicamente en FTS5
	_, err = tx.Exec("INSERT INTO observations_fts (observation_id, entity_name, content) VALUES (?, ?, ?)", obsID, entityName, content)
	if err != nil {
		return 0, fmt.Errorf("error al indexar observación en FTS5: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("error al comprometer transacción de observación: %w", err)
	}

	return obsID, nil
}

// LinkEntities establece una relación semántica direccional entre dos entidades (ej. "Go" -> "USES" -> "Generics").
func (s *Storage) LinkEntities(sourceName, targetName, relationType, project string) error {
	sourceName = strings.TrimSpace(sourceName)
	targetName = strings.TrimSpace(targetName)
	relationType = strings.TrimSpace(strings.ToUpper(relationType))
	project = strings.TrimSpace(project)

	if sourceName == "" || targetName == "" || relationType == "" {
		return fmt.Errorf("los parámetros de relación no pueden estar vacíos")
	}
	if project == "" {
		project = "global"
	}

	sourceID, err := s.SaveEntity(sourceName, "concept", project)
	if err != nil {
		return fmt.Errorf("error al resolver entidad origen: %w", err)
	}

	targetID, err := s.SaveEntity(targetName, "concept", project)
	if err != nil {
		return fmt.Errorf("error al resolver entidad destino: %w", err)
	}

	// Insert OR Ignore para evitar violaciones de clave primaria si ya existe el enlace
	_, err = s.db.Exec(`
		INSERT OR IGNORE INTO relations (source_id, target_id, relation_type, project)
		VALUES (?, ?, ?, ?)
	`, sourceID, targetID, relationType, project)
	if err != nil {
		return fmt.Errorf("error al guardar relación semántica: %w", err)
	}

	return nil
}

// SearchBrain realiza una búsqueda de texto completo con BM25 en observaciones y le inyecta las relaciones conectadas.
func (s *Storage) SearchBrain(query, project string, limit int) ([]BrainSearchResult, error) {
	cleanedQuery := sanitizeQuery(query)
	if cleanedQuery == "" {
		return nil, nil
	}
	project = strings.TrimSpace(project)
	if project == "" {
		project = "global"
	}

	sqlQuery := `
		SELECT 
			o.id, 
			e.name, 
			e.category, 
			o.content, 
			COALESCE(o.source_url, ''),
			snippet(observations_fts, 2, '***', '***', '...', 32) as snippet, 
			rank
		FROM observations_fts
		JOIN observations o ON o.id = observations_fts.observation_id
		JOIN entities e ON e.id = o.entity_id
		WHERE (o.project = ? OR o.project = 'global') AND observations_fts MATCH ?
		ORDER BY rank ASC
		LIMIT ?;
	`

	rows, err := s.db.Query(sqlQuery, project, cleanedQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("error en la búsqueda FTS5 del cerebro con query '%s' (sanitizado: '%s'): %w", query, cleanedQuery, err)
	}
	defer rows.Close()

	var results []BrainSearchResult
	for rows.Next() {
		var res BrainSearchResult
		if err := rows.Scan(&res.ObservationID, &res.EntityName, &res.EntityCategory, &res.Content, &res.SourceURL, &res.Snippet, &res.Rank); err != nil {
			return nil, fmt.Errorf("error al escanear resultado del cerebro: %w", err)
		}

		// Obtener relaciones semánticas legibles asociadas a esta entidad
		relations, err := s.getEntityRelations(res.EntityName, project)
		if err == nil {
			res.Relations = relations
		}

		results = append(results, res)
	}

	return results, nil
}

// getEntityRelations obtiene todos los enlaces semánticos donde esta entidad participa.
func (s *Storage) getEntityRelations(entityName, project string) ([]string, error) {
	query := `
		SELECT r.relation_type, e2.name
		FROM relations r
		JOIN entities e1 ON e1.id = r.source_id
		JOIN entities e2 ON e2.id = r.target_id
		WHERE LOWER(e1.name) = LOWER(?) AND (r.project = ? OR r.project = 'global')
	`
	rows, err := s.db.Query(query, entityName, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relations []string
	for rows.Next() {
		var relType, targetName string
		if err := rows.Scan(&relType, &targetName); err == nil {
			relations = append(relations, fmt.Sprintf("%s %s", relType, targetName))
		}
	}
	return relations, nil
}

// === MÉTODOS DE LA FASE 2: JANITOR (DEDUPLICACIÓN Y AUTOAPRENDIZAJE) ===

// Deduplicate busca observaciones redundantes pertenecientes a la misma entidad,
// calcula la similitud de palabras usando Jaccard y elimina las duplicadas quedándose con la más larga/descriptiva.
func (s *Storage) Deduplicate() (int, error) {
	rows, err := s.db.Query("SELECT id, entity_id, content FROM observations ORDER BY entity_id")
	if err != nil {
		return 0, fmt.Errorf("error al consultar observaciones para deduplicación: %w", err)
	}
	defer rows.Close()

	type obsItem struct {
		id       int64
		entityID int64
		content  string
	}

	byEntity := make(map[int64][]obsItem)
	for rows.Next() {
		var item obsItem
		if err := rows.Scan(&item.id, &item.entityID, &item.content); err != nil {
			return 0, fmt.Errorf("error al escanear observación: %w", err)
		}
		byEntity[item.entityID] = append(byEntity[item.entityID], item)
	}
	rows.Close()

	var toDelete []int64

	for _, list := range byEntity {
		if len(list) < 2 {
			continue
		}
		deletedInGroup := make(map[int64]bool)
		for i := 0; i < len(list); i++ {
			if deletedInGroup[list[i].id] {
				continue
			}
			for j := i + 1; j < len(list); j++ {
				if deletedInGroup[list[j].id] {
					continue
				}
				sim := jaccardSimilarity(list[i].content, list[j].content)
				if sim >= 0.7 { // Umbral de 70% de similitud de tokens
					if len(list[i].content) >= len(list[j].content) {
						deletedInGroup[list[j].id] = true
						toDelete = append(toDelete, list[j].id)
					} else {
						deletedInGroup[list[i].id] = true
						toDelete = append(toDelete, list[i].id)
						break
					}
				}
			}
		}
	}

	if len(toDelete) == 0 {
		return 0, nil
	}

	// Iniciamos una transacción atómica para eliminar las observaciones y limpiar el índice FTS5
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("error al iniciar transacción de deduplicación: %w", err)
	}
	defer tx.Rollback()

	delFTS, err := tx.Prepare("DELETE FROM observations_fts WHERE observation_id = ?")
	if err != nil {
		return 0, fmt.Errorf("error al preparar delete de FTS5: %w", err)
	}
	defer delFTS.Close()

	delObs, err := tx.Prepare("DELETE FROM observations WHERE id = ?")
	if err != nil {
		return 0, fmt.Errorf("error al preparar delete de observaciones: %w", err)
	}
	defer delObs.Close()

	for _, id := range toDelete {
		if _, err := delFTS.Exec(id); err != nil {
			return 0, fmt.Errorf("error al eliminar de FTS5 observation_id %d: %w", id, err)
		}
		if _, err := delObs.Exec(id); err != nil {
			return 0, fmt.Errorf("error al eliminar de observaciones id %d: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("error al comprometer transacción de deduplicación: %w", err)
	}

	return len(toDelete), nil
}

// jaccardSimilarity calcula el coeficiente de Jaccard entre dos cadenas basándose en tokens únicos de palabras significativas.
func jaccardSimilarity(s1, s2 string) float64 {
	w1 := tokenizeWords(s1)
	w2 := tokenizeWords(s2)

	if len(w1) == 0 || len(w2) == 0 {
		return 0.0
	}

	intersection := 0
	for w := range w1 {
		if w2[w] {
			intersection++
		}
	}

	union := len(w1) + len(w2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// tokenizeWords descompone un texto en un mapa de palabras en minúscula, limpiando puntuación y omitiendo stopwords básicas.
func tokenizeWords(s string) map[string]bool {
	words := make(map[string]bool)
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true, "in": true, "on": true, "at": true, "to": true, "for": true, "of": true, "with": true, "by": true,
		"el": true, "la": true, "los": true, "las": true, "un": true, "una": true, "unos": true, "unas": true, "y": true, "o": true, "pero": true, "en": true, "de": true, "con": true, "por": true, "para": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
		"es": true, "son": true, "era": true, "eran": true, "ser": true, "sido": true,
	}

	s = strings.ToLower(s)
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(' ')
		}
	}

	fields := strings.Fields(sb.String())
	for _, field := range fields {
		if len(field) > 1 && !stopwords[field] {
			words[field] = true
		}
	}
	return words
}

// AutoLink analiza las observaciones buscando verbos y expresiones de transición semántica.
// Mapea el destino y crea enlaces semánticos automáticos en SQLite.
func (s *Storage) AutoLink() (int, error) {
	query := `
		SELECT o.id, e.name, o.content, o.project
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return 0, fmt.Errorf("error al consultar observaciones para autoaprendizaje: %w", err)
	}
	defer rows.Close()

	type obsRow struct {
		id         int64
		entityName string
		content    string
		project    string
	}

	var list []obsRow
	for rows.Next() {
		var r obsRow
		if err := rows.Scan(&r.id, &r.entityName, &r.content, &r.project); err != nil {
			return 0, fmt.Errorf("error al escanear observación: %w", err)
		}
		list = append(list, r)
	}
	rows.Close()

	type triggerMap struct {
		phrase string
		rel    string
	}
	triggers := []triggerMap{
		{"reemplazado por", "DEPRECATED_BY"},
		{"compatible with", "COMPATIBLE_WITH"},
		{"compatible con", "COMPATIBLE_WITH"},
		{"deprecated by", "DEPRECATED_BY"},
		{"deprecado por", "DEPRECATED_BY"},
		{"replaced by", "DEPRECATED_BY"},
		{"depends on", "DEPENDS_ON"},
		{"depende de", "DEPENDS_ON"},
		{"uses", "USES"},
		{"usa", "USES"},
	}

	linkedCount := 0
	for _, item := range list {
		contentLower := strings.ToLower(item.content)
		for _, t := range triggers {
			if strings.Contains(contentLower, t.phrase) {
				target := parseRelationTarget(item.content, t.phrase)
				if target != "" && strings.ToLower(item.entityName) != strings.ToLower(target) {
					err := s.LinkEntities(item.entityName, target, t.rel, item.project)
					if err == nil {
						linkedCount++
					}
				}
			}
		}
	}

	return linkedCount, nil
}

// parseRelationTarget extrae e identifica el nombre de la entidad destino a partir del contenido
// de una observación y una frase gatillo, aplicando heurísticas avanzadas para ignorar conjunciones.
func parseRelationTarget(content string, trigger string) string {
	idx := strings.Index(strings.ToLower(content), trigger)
	if idx == -1 {
		return ""
	}

	after := content[idx+len(trigger):]
	after = strings.TrimSpace(after)
	if after == "" {
		return ""
	}

	fields := strings.Fields(after)
	if len(fields) == 0 {
		return ""
	}

	stopWords := map[string]bool{
		"for": true, "and": true, "with": true, "by": true, "to": true, "in": true, "on": true, "at": true,
		"para": true, "y": true, "con": true, "por": true, "en": true, "de": true, "que": true, "el": true, "la": true,
	}

	var targetWords []string
	for i, word := range fields {
		if i >= 3 { // Máximo de 3 palabras para capturar nombres compuestos tipo "Go 1.26"
			break
		}

		cleanedWord := word
		hasPunctuationEnd := false
		if strings.HasSuffix(word, ".") || strings.HasSuffix(word, ",") || strings.HasSuffix(word, ";") || strings.HasSuffix(word, ":") {
			cleanedWord = word[:len(word)-1]
			hasPunctuationEnd = true
		}

		if cleanedWord == "" || stopWords[strings.ToLower(cleanedWord)] {
			break
		}

		targetWords = append(targetWords, cleanedWord)

		if hasPunctuationEnd {
			break
		}
	}

	if len(targetWords) == 0 {
		return ""
	}

	return strings.Join(targetWords, " ")
}

// === MÉTODOS DE SOPORTE COMPARTIDOS ===

// sanitizeQuery protege la consulta de búsqueda para evitar que caracteres especiales de FTS5
// (como puntos, dos puntos, asteriscos o guiones) rompan el parser de SQLite.
func sanitizeQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}

	if strings.HasPrefix(q, `"`) && strings.HasSuffix(q, `"`) {
		return q
	}

	if strings.ContainsAny(q, ".:-*") {
		escaped := strings.ReplaceAll(q, `"`, `""`)
		return `"` + escaped + `"`
	}

	return q
}

// Close cierra la conexión a SQLite.
func (s *Storage) Close() error {
	return s.db.Close()
}
