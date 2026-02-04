# graph package

Neo4j graph database operations for code relationships.

## Purpose

Store and query code relationships (imports, calls, extends) for graph-based search expansion and related file discovery.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Neo4jStore` | Neo4j client wrapper | `neo4j.go:17-19` |
| `Repository` | Repository node | `neo4j.go:38-41` |
| `Module` | Module node | `neo4j.go:44-49` |
| `File` | File node | `neo4j.go:52-58` |
| `Symbol` | Symbol node | `neo4j.go:61-70` |
| `Pattern` | Pattern node | `neo4j.go:73-78` |
| `Relationship` | Edge between nodes | `neo4j.go:81-86` |

## Graph Schema

```
(:Repository)-[:CONTAINS]->(:Module)
(:Module)-[:DEPENDS_ON]->(:Module)
(:File)-[:IMPORTS]->(:File)
(:File)-[:CONTAINS]->(:Symbol)
(:Symbol)-[:CALLS]->(:Symbol)
(:Symbol)-[:EXTENDS]->(:Symbol)
(:Pattern)-[:FOLLOWED_BY]->(:File)
```

## Usage

```go
store, err := graph.NewNeo4jStore("bolt://localhost:7687", "neo4j", "password")
defer store.Close(ctx)

err = store.EnsureSchema(ctx)
err = store.UpsertFile(ctx, graph.File{...})
err = store.UpsertSymbol(ctx, graph.Symbol{...})
err = store.CreateCallRelationship(ctx, repo, caller, callee)

related, err := store.FindRelatedFiles(ctx, repo, path, 10)
callers, err := store.FindCallers(ctx, repo, symbolName)
```

## Methods

| Method | Description |
|--------|-------------|
| `EnsureSchema(ctx)` | Create indexes/constraints |
| `UpsertRepository(ctx, repo)` | Create/update repository |
| `UpsertModule(ctx, module)` | Create/update module |
| `UpsertFile(ctx, file)` | Create/update file |
| `UpsertSymbol(ctx, symbol)` | Create/update symbol |
| `CreateImportRelationship(ctx, repo, src, tgt)` | File imports file |
| `CreateCallRelationship(ctx, repo, caller, callee)` | Symbol calls symbol |
| `CreateExtendsRelationship(ctx, repo, child, parent)` | Symbol extends symbol |
| `FindSymbolByName(ctx, repo, name)` | Find symbols by name |
| `FindCallers(ctx, repo, name)` | Find callers of symbol |
| `FindCallees(ctx, repo, name)` | Find callees of symbol |
| `FindRelatedFiles(ctx, repo, path, limit)` | Find related files |
| `ExpandFromSymbols(ctx, repo, names, depth, limit)` | Graph expansion |
| `GetFileHash(ctx, repo, path)` | Get stored file hash |
| `GetAllFileHashes(ctx, repo)` | Get all file hashes |
| `DeleteFile(ctx, repo, path)` | Delete file and symbols |
| `DeleteRepository(ctx, name)` | Delete repo and all nodes |

## Incremental Indexing

Use `GetFileHash()` and `GetAllFileHashes()` to compare current file hashes with stored hashes for incremental updates.

## Connection

| Variable | Default | Description |
|----------|---------|-------------|
| `NEO4J_URL` | `bolt://localhost:7687` | Neo4j Bolt endpoint |
| `NEO4J_USER` | `neo4j` | Username |
| `NEO4J_PASSWORD` | - | Password |

## Gotchas

1. **Schema first**: Call `EnsureSchema()` before operations
2. **APOC optional**: `ExpandFromSymbols()` falls back without APOC
3. **Relationship direction**: IMPORTS/CALLS/EXTENDS have semantic direction
4. **Unique constraints**: File uniqueness is (repo, path), Symbol is (repo, file_path, name, start_line)
