// Package graph provides Neo4j graph database operations for code relationships.
package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Neo4jStore handles graph storage in Neo4j.
type Neo4jStore struct {
	driver neo4j.DriverWithContext
}

// Node types in the graph
const (
	NodeRepository = "Repository"
	NodeModule     = "Module"
	NodeFile       = "File"
	NodeSymbol     = "Symbol"
	NodePattern    = "Pattern"
	NodeAgentsDoc  = "AgentsDoc"
	NodeDocChunk   = "DocChunk"
)

// Relationship types
const (
	RelContains   = "CONTAINS"
	RelImports    = "IMPORTS"
	RelCalls      = "CALLS"
	RelExtends    = "EXTENDS"
	RelDependsOn  = "DEPENDS_ON"
	RelDescribes  = "DESCRIBES"
	RelMentions   = "MENTIONS"
	RelReferences = "REFERENCES"
	RelFollowedBy = "FOLLOWED_BY"
)

// Repository represents a code repository node.
type Repository struct {
	Name string
	Path string
}

// Module represents a module within a repository.
type Module struct {
	Repo        string
	Path        string // e.g., "fisio.imports"
	FSPath      string // e.g., "fisio/fisio/imports/"
	Description string
}

// File represents a source file node.
type File struct {
	Path        string
	Repo        string
	ModuleRoot  string
	Hash        string
	LastIndexed time.Time
}

// Symbol represents a code symbol (function, class, method).
type Symbol struct {
	Name      string
	Kind      string // function, class, method
	Repo      string
	FilePath  string
	StartLine int
	EndLine   int
	Signature string
}

// Pattern represents a code pattern.
type Pattern struct {
	Name          string
	Module        string
	CanonicalFile string
	MemberCount   int
}

// Relationship represents an edge between nodes.
type Relationship struct {
	Type       string
	SourceID   string
	TargetID   string
	Properties map[string]interface{}
}

// NewNeo4jStore creates a new Neo4j store.
func NewNeo4jStore(uri, username, password string) (*Neo4jStore, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create Neo4j driver: %w", err)
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("failed to connect to Neo4j: %w", err)
	}

	return &Neo4jStore{driver: driver}, nil
}

// Close closes the Neo4j driver.
func (s *Neo4jStore) Close(ctx context.Context) error {
	return s.driver.Close(ctx)
}

// EnsureSchema creates indexes and constraints.
func (s *Neo4jStore) EnsureSchema(ctx context.Context) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	constraints := []string{
		// Unique constraints
		"CREATE CONSTRAINT repo_name IF NOT EXISTS FOR (r:Repository) REQUIRE r.name IS UNIQUE",
		"CREATE CONSTRAINT file_path IF NOT EXISTS FOR (f:File) REQUIRE (f.repo, f.path) IS UNIQUE",
		"CREATE CONSTRAINT symbol_id IF NOT EXISTS FOR (s:Symbol) REQUIRE (s.repo, s.file_path, s.name, s.start_line) IS UNIQUE",
		"CREATE CONSTRAINT module_path IF NOT EXISTS FOR (m:Module) REQUIRE (m.repo, m.path) IS UNIQUE",
		"CREATE CONSTRAINT pattern_name IF NOT EXISTS FOR (p:Pattern) REQUIRE (p.module, p.name) IS UNIQUE",
	}

	indexes := []string{
		// Performance indexes
		"CREATE INDEX file_repo IF NOT EXISTS FOR (f:File) ON (f.repo)",
		"CREATE INDEX file_hash IF NOT EXISTS FOR (f:File) ON (f.hash)",
		"CREATE INDEX symbol_repo IF NOT EXISTS FOR (s:Symbol) ON (s.repo)",
		"CREATE INDEX symbol_kind IF NOT EXISTS FOR (s:Symbol) ON (s.kind)",
		"CREATE INDEX symbol_name IF NOT EXISTS FOR (s:Symbol) ON (s.name)",
		"CREATE INDEX module_repo IF NOT EXISTS FOR (m:Module) ON (m.repo)",
	}

	// Create constraints
	for _, constraint := range constraints {
		_, err := session.Run(ctx, constraint, nil)
		if err != nil {
			return fmt.Errorf("failed to create constraint: %w", err)
		}
	}

	// Create indexes
	for _, index := range indexes {
		_, err := session.Run(ctx, index, nil)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// UpsertRepository creates or updates a repository node.
func (s *Neo4jStore) UpsertRepository(ctx context.Context, repo Repository) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MERGE (r:Repository {name: $name})
		SET r.path = $path
	`, map[string]interface{}{
		"name": repo.Name,
		"path": repo.Path,
	})

	return err
}

// UpsertModule creates or updates a module node.
func (s *Neo4jStore) UpsertModule(ctx context.Context, module Module) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MERGE (m:Module {repo: $repo, path: $path})
		SET m.fs_path = $fs_path, m.description = $description
		WITH m
		MATCH (r:Repository {name: $repo})
		MERGE (r)-[:CONTAINS]->(m)
	`, map[string]interface{}{
		"repo":        module.Repo,
		"path":        module.Path,
		"fs_path":     module.FSPath,
		"description": module.Description,
	})

	return err
}

// UpsertFile creates or updates a file node.
func (s *Neo4jStore) UpsertFile(ctx context.Context, file File) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MERGE (f:File {repo: $repo, path: $path})
		SET f.module_root = $module_root,
		    f.hash = $hash,
		    f.last_indexed = $last_indexed
	`, map[string]interface{}{
		"repo":         file.Repo,
		"path":         file.Path,
		"module_root":  file.ModuleRoot,
		"hash":         file.Hash,
		"last_indexed": file.LastIndexed.Unix(),
	})

	return err
}

// UpsertSymbol creates or updates a symbol node.
func (s *Neo4jStore) UpsertSymbol(ctx context.Context, symbol Symbol) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MERGE (s:Symbol {repo: $repo, file_path: $file_path, name: $name, start_line: $start_line})
		SET s.kind = $kind,
		    s.end_line = $end_line,
		    s.signature = $signature
		WITH s
		MATCH (f:File {repo: $repo, path: $file_path})
		MERGE (f)-[:CONTAINS]->(s)
	`, map[string]interface{}{
		"repo":       symbol.Repo,
		"file_path":  symbol.FilePath,
		"name":       symbol.Name,
		"start_line": symbol.StartLine,
		"kind":       symbol.Kind,
		"end_line":   symbol.EndLine,
		"signature":  symbol.Signature,
	})

	return err
}

// UpsertPattern creates or updates a pattern node.
func (s *Neo4jStore) UpsertPattern(ctx context.Context, pattern Pattern) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MERGE (p:Pattern {module: $module, name: $name})
		SET p.canonical_file = $canonical_file,
		    p.member_count = $member_count
	`, map[string]interface{}{
		"module":         pattern.Module,
		"name":           pattern.Name,
		"canonical_file": pattern.CanonicalFile,
		"member_count":   pattern.MemberCount,
	})

	return err
}

// CreateRelationship creates an edge between nodes.
func (s *Neo4jStore) CreateRelationship(ctx context.Context, rel Relationship) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	// Build dynamic relationship creation based on type
	var query string
	params := map[string]interface{}{
		"source_id": rel.SourceID,
		"target_id": rel.TargetID,
	}

	switch rel.Type {
	case RelImports:
		// File imports File
		query = `
			MATCH (source:File {path: $source_id})
			MATCH (target:File {path: $target_id})
			WHERE source.repo = target.repo
			MERGE (source)-[:IMPORTS]->(target)
		`
	case RelCalls:
		// Symbol calls Symbol
		query = `
			MATCH (source:Symbol)
			WHERE source.file_path + ':' + toString(source.start_line) = $source_id
			MATCH (target:Symbol)
			WHERE target.file_path + ':' + toString(target.start_line) = $target_id
			MERGE (source)-[:CALLS]->(target)
		`
	case RelExtends:
		// Symbol extends Symbol
		query = `
			MATCH (source:Symbol)
			WHERE source.file_path + ':' + toString(source.start_line) = $source_id
			MATCH (target:Symbol)
			WHERE target.file_path + ':' + toString(target.start_line) = $target_id
			MERGE (source)-[:EXTENDS]->(target)
		`
	case RelDependsOn:
		// Module depends on Module
		query = `
			MATCH (source:Module {path: $source_id})
			MATCH (target:Module {path: $target_id})
			WHERE source.repo = target.repo
			MERGE (source)-[:DEPENDS_ON]->(target)
		`
	case RelFollowedBy:
		// Pattern followed by File
		query = `
			MATCH (p:Pattern {name: $source_id})
			MATCH (f:File {path: $target_id})
			MERGE (p)-[:FOLLOWED_BY]->(f)
		`
	default:
		return fmt.Errorf("unknown relationship type: %s", rel.Type)
	}

	_, err := session.Run(ctx, query, params)
	return err
}

// CreateImportRelationship creates an IMPORTS relationship between files.
func (s *Neo4jStore) CreateImportRelationship(ctx context.Context, repo, sourcePath, targetPath string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MATCH (source:File {repo: $repo, path: $source_path})
		MATCH (target:File {repo: $repo, path: $target_path})
		MERGE (source)-[:IMPORTS]->(target)
	`, map[string]interface{}{
		"repo":        repo,
		"source_path": sourcePath,
		"target_path": targetPath,
	})

	return err
}

// CreateCallRelationship creates a CALLS relationship between symbols.
func (s *Neo4jStore) CreateCallRelationship(ctx context.Context, repo string, caller, callee Symbol) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MATCH (caller:Symbol {repo: $repo, file_path: $caller_file, name: $caller_name, start_line: $caller_line})
		MATCH (callee:Symbol {repo: $repo, name: $callee_name})
		MERGE (caller)-[:CALLS]->(callee)
	`, map[string]interface{}{
		"repo":        repo,
		"caller_file": caller.FilePath,
		"caller_name": caller.Name,
		"caller_line": caller.StartLine,
		"callee_name": callee.Name,
	})

	return err
}

// CreateExtendsRelationship creates an EXTENDS relationship between symbols.
func (s *Neo4jStore) CreateExtendsRelationship(ctx context.Context, repo string, child, parent Symbol) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MATCH (child:Symbol {repo: $repo, file_path: $child_file, name: $child_name, start_line: $child_line})
		MATCH (parent:Symbol {repo: $repo, name: $parent_name})
		MERGE (child)-[:EXTENDS]->(parent)
	`, map[string]interface{}{
		"repo":        repo,
		"child_file":  child.FilePath,
		"child_name":  child.Name,
		"child_line":  child.StartLine,
		"parent_name": parent.Name,
	})

	return err
}

// GetFileByHash returns a file by its content hash.
func (s *Neo4jStore) GetFileByHash(ctx context.Context, repo, hash string) (*File, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (f:File {repo: $repo, hash: $hash})
		RETURN f.path, f.module_root, f.hash, f.last_indexed
	`, map[string]interface{}{
		"repo": repo,
		"hash": hash,
	})
	if err != nil {
		return nil, err
	}

	if result.Next(ctx) {
		record := result.Record()
		lastIndexed, _ := record.Get("f.last_indexed")
		return &File{
			Path:        record.Values[0].(string),
			Repo:        repo,
			ModuleRoot:  record.Values[1].(string),
			Hash:        record.Values[2].(string),
			LastIndexed: time.Unix(lastIndexed.(int64), 0),
		}, nil
	}

	return nil, nil
}

// GetFileHash returns the stored hash for a file path.
func (s *Neo4jStore) GetFileHash(ctx context.Context, repo, path string) (string, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (f:File {repo: $repo, path: $path})
		RETURN f.hash
	`, map[string]interface{}{
		"repo": repo,
		"path": path,
	})
	if err != nil {
		return "", err
	}

	if result.Next(ctx) {
		hash, _ := result.Record().Get("f.hash")
		if hash != nil {
			return hash.(string), nil
		}
	}

	return "", nil
}

// FindSymbolByName finds symbols matching a name.
func (s *Neo4jStore) FindSymbolByName(ctx context.Context, repo, name string) ([]Symbol, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (s:Symbol {repo: $repo, name: $name})
		RETURN s.name, s.kind, s.file_path, s.start_line, s.end_line, s.signature
	`, map[string]interface{}{
		"repo": repo,
		"name": name,
	})
	if err != nil {
		return nil, err
	}

	var symbols []Symbol
	for result.Next(ctx) {
		record := result.Record()
		symbols = append(symbols, Symbol{
			Name:      getString(record, "s.name"),
			Kind:      getString(record, "s.kind"),
			Repo:      repo,
			FilePath:  getString(record, "s.file_path"),
			StartLine: getInt(record, "s.start_line"),
			EndLine:   getInt(record, "s.end_line"),
			Signature: getString(record, "s.signature"),
		})
	}

	return symbols, nil
}

// FindCallers finds symbols that call the given symbol.
func (s *Neo4jStore) FindCallers(ctx context.Context, repo, symbolName string) ([]Symbol, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (caller:Symbol)-[:CALLS]->(callee:Symbol {repo: $repo, name: $name})
		RETURN caller.name, caller.kind, caller.file_path, caller.start_line, caller.end_line, caller.signature
	`, map[string]interface{}{
		"repo": repo,
		"name": symbolName,
	})
	if err != nil {
		return nil, err
	}

	var symbols []Symbol
	for result.Next(ctx) {
		record := result.Record()
		symbols = append(symbols, Symbol{
			Name:      getString(record, "caller.name"),
			Kind:      getString(record, "caller.kind"),
			Repo:      repo,
			FilePath:  getString(record, "caller.file_path"),
			StartLine: getInt(record, "caller.start_line"),
			EndLine:   getInt(record, "caller.end_line"),
			Signature: getString(record, "caller.signature"),
		})
	}

	return symbols, nil
}

// FindCallees finds symbols called by the given symbol.
func (s *Neo4jStore) FindCallees(ctx context.Context, repo, symbolName string) ([]Symbol, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (caller:Symbol {repo: $repo, name: $name})-[:CALLS]->(callee:Symbol)
		RETURN callee.name, callee.kind, callee.file_path, callee.start_line, callee.end_line, callee.signature
	`, map[string]interface{}{
		"repo": repo,
		"name": symbolName,
	})
	if err != nil {
		return nil, err
	}

	var symbols []Symbol
	for result.Next(ctx) {
		record := result.Record()
		symbols = append(symbols, Symbol{
			Name:      getString(record, "callee.name"),
			Kind:      getString(record, "callee.kind"),
			Repo:      repo,
			FilePath:  getString(record, "callee.file_path"),
			StartLine: getInt(record, "callee.start_line"),
			EndLine:   getInt(record, "callee.end_line"),
			Signature: getString(record, "callee.signature"),
		})
	}

	return symbols, nil
}

// FindRelatedFiles finds files related to the given file via imports or shared symbols.
func (s *Neo4jStore) FindRelatedFiles(ctx context.Context, repo, filePath string, limit int) ([]File, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (f:File {repo: $repo, path: $path})
		OPTIONAL MATCH (f)-[:IMPORTS]->(imported:File)
		OPTIONAL MATCH (importer:File)-[:IMPORTS]->(f)
		OPTIONAL MATCH (f)-[:CONTAINS]->(s:Symbol)-[:CALLS]->(callee:Symbol)<-[:CONTAINS]-(callee_file:File)
		OPTIONAL MATCH (f)-[:CONTAINS]->(s2:Symbol)<-[:CALLS]-(caller:Symbol)<-[:CONTAINS]-(caller_file:File)
		WITH COLLECT(DISTINCT imported) + COLLECT(DISTINCT importer) + COLLECT(DISTINCT callee_file) + COLLECT(DISTINCT caller_file) AS related
		UNWIND related AS r
		WITH DISTINCT r
		WHERE r IS NOT NULL
		RETURN r.path, r.repo, r.module_root, r.hash, r.last_indexed
		LIMIT $limit
	`, map[string]interface{}{
		"repo":  repo,
		"path":  filePath,
		"limit": limit,
	})
	if err != nil {
		return nil, err
	}

	var files []File
	for result.Next(ctx) {
		record := result.Record()
		lastIndexed := getInt64(record, "r.last_indexed")
		files = append(files, File{
			Path:        getString(record, "r.path"),
			Repo:        getString(record, "r.repo"),
			ModuleRoot:  getString(record, "r.module_root"),
			Hash:        getString(record, "r.hash"),
			LastIndexed: time.Unix(lastIndexed, 0),
		})
	}

	return files, nil
}

// ExpandFromSymbols returns related symbols via graph traversal.
func (s *Neo4jStore) ExpandFromSymbols(ctx context.Context, repo string, symbolNames []string, depth int, limit int) ([]Symbol, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (s:Symbol)
		WHERE s.repo = $repo AND s.name IN $names
		CALL apoc.path.subgraphNodes(s, {
			relationshipFilter: "CALLS|EXTENDS|CONTAINS",
			minLevel: 1,
			maxLevel: $depth,
			limit: $limit
		}) YIELD node
		WHERE node:Symbol
		RETURN DISTINCT node.name, node.kind, node.file_path, node.start_line, node.end_line, node.signature
	`, map[string]interface{}{
		"repo":  repo,
		"names": symbolNames,
		"depth": depth,
		"limit": limit,
	})
	if err != nil {
		// Fallback if APOC not available
		return s.expandFromSymbolsBasic(ctx, repo, symbolNames, limit)
	}

	var symbols []Symbol
	for result.Next(ctx) {
		record := result.Record()
		symbols = append(symbols, Symbol{
			Name:      getString(record, "node.name"),
			Kind:      getString(record, "node.kind"),
			Repo:      repo,
			FilePath:  getString(record, "node.file_path"),
			StartLine: getInt(record, "node.start_line"),
			EndLine:   getInt(record, "node.end_line"),
			Signature: getString(record, "node.signature"),
		})
	}

	return symbols, nil
}

// expandFromSymbolsBasic is a fallback without APOC.
func (s *Neo4jStore) expandFromSymbolsBasic(ctx context.Context, repo string, symbolNames []string, limit int) ([]Symbol, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (s:Symbol)
		WHERE s.repo = $repo AND s.name IN $names
		OPTIONAL MATCH (s)-[:CALLS]->(callee:Symbol)
		OPTIONAL MATCH (caller:Symbol)-[:CALLS]->(s)
		OPTIONAL MATCH (s)-[:EXTENDS]->(parent:Symbol)
		OPTIONAL MATCH (child:Symbol)-[:EXTENDS]->(s)
		WITH COLLECT(DISTINCT callee) + COLLECT(DISTINCT caller) + COLLECT(DISTINCT parent) + COLLECT(DISTINCT child) AS related
		UNWIND related AS r
		WITH DISTINCT r
		WHERE r IS NOT NULL
		RETURN r.name, r.kind, r.file_path, r.start_line, r.end_line, r.signature
		LIMIT $limit
	`, map[string]interface{}{
		"repo":  repo,
		"names": symbolNames,
		"limit": limit,
	})
	if err != nil {
		return nil, err
	}

	var symbols []Symbol
	for result.Next(ctx) {
		record := result.Record()
		symbols = append(symbols, Symbol{
			Name:      getString(record, "r.name"),
			Kind:      getString(record, "r.kind"),
			Repo:      repo,
			FilePath:  getString(record, "r.file_path"),
			StartLine: getInt(record, "r.start_line"),
			EndLine:   getInt(record, "r.end_line"),
			Signature: getString(record, "r.signature"),
		})
	}

	return symbols, nil
}

// DeleteRepository removes a repository and all its related nodes.
func (s *Neo4jStore) DeleteRepository(ctx context.Context, repoName string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MATCH (r:Repository {name: $name})
		OPTIONAL MATCH (r)-[*]->(n)
		DETACH DELETE r, n
	`, map[string]interface{}{
		"name": repoName,
	})

	return err
}

// DeleteFile removes a file and its symbols.
func (s *Neo4jStore) DeleteFile(ctx context.Context, repo, path string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.Run(ctx, `
		MATCH (f:File {repo: $repo, path: $path})
		OPTIONAL MATCH (f)-[:CONTAINS]->(s:Symbol)
		DETACH DELETE f, s
	`, map[string]interface{}{
		"repo": repo,
		"path": path,
	})

	return err
}

// GetAllFileHashes returns all file hashes for a repository.
func (s *Neo4jStore) GetAllFileHashes(ctx context.Context, repo string) (map[string]string, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (f:File {repo: $repo})
		RETURN f.path, f.hash
	`, map[string]interface{}{
		"repo": repo,
	})
	if err != nil {
		return nil, err
	}

	hashes := make(map[string]string)
	for result.Next(ctx) {
		record := result.Record()
		path := getString(record, "f.path")
		hash := getString(record, "f.hash")
		if path != "" && hash != "" {
			hashes[path] = hash
		}
	}

	return hashes, nil
}

// Helper functions for extracting values from records
func getString(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

func getInt(record *neo4j.Record, key string) int {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func getInt64(record *neo4j.Record, key string) int64 {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return 0
	}
	if i, ok := val.(int64); ok {
		return i
	}
	return 0
}
