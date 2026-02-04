// Package chunk provides types and extraction for indexable code chunks.
package chunk

// ChunkType distinguishes code from documentation.
type ChunkType string

const (
	ChunkTypeCode ChunkType = "code"
	ChunkTypeDoc  ChunkType = "doc"
)

// Chunk represents an indexable unit of code or documentation.
type Chunk struct {
	// Identity
	ID       string `json:"id"` // Generated: hash of repo+path+symbol+lines
	Repo     string `json:"repo"`
	FilePath string `json:"file_path"`

	// Location
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`

	// Classification
	Type        ChunkType `json:"type"`              // code | doc
	Kind        string    `json:"kind,omitempty"`    // function | class | method | pattern
	ModulePath  string    `json:"module_path"`       // fisio.imports.aws
	ModuleRoot  string    `json:"module_root"`       // fisio
	Submodule   string    `json:"submodule"`         // imports
	SymbolName  string    `json:"symbol_name,omitempty"`
	HeadingPath string    `json:"heading_path,omitempty"` // For docs

	// Content
	Content       string `json:"content"`
	ContextHeader string `json:"context_header,omitempty"` // Injected context for methods
	Signature     string `json:"signature,omitempty"`
	Docstring     string `json:"docstring,omitempty"`

	// Metadata
	IsTest          bool    `json:"is_test"`
	RetrievalWeight float32 `json:"retrieval_weight"` // 1.0 normal, 0.5 for tests
	HasSecrets      bool    `json:"has_secrets"`
	FollowsPattern  string  `json:"follows_pattern,omitempty"`

	// Vector (populated after embedding)
	Vector []float32 `json:"vector,omitempty"`

	// Score (populated by search, not stored)
	Score float32 `json:"-"`
}

// TokenEstimate returns rough token count for the chunk.
func (c *Chunk) TokenEstimate() int {
	// Rough estimate: ~4 chars per token
	return len(c.Content) / 4
}

// GenerateID creates a deterministic ID for a chunk.
func GenerateID(repo, filePath, symbolName string, startLine int) string {
	return generateChunkID(repo, filePath, symbolName, startLine)
}
