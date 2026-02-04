// Package docs provides parsing for documentation files like AGENTS.md.
package docs

import (
	"regexp"
	"strings"

	"github.com/randalmurphy/ai-devtools-admin/internal/chunk"
)

// AgentsDoc represents a parsed AGENTS.md file.
type AgentsDoc struct {
	Path             string
	Repo             string
	Module           string
	Title            string
	Description      string
	EntryPoints      []string
	MentionedSymbols []string
	MentionedFiles   []string
	Sections         []Section
}

// Section represents a section of the document.
type Section struct {
	Heading     string
	HeadingPath string // Full path: "Key Patterns > Import Pattern"
	Level       int
	Content     string
	StartLine   int
	EndLine     int
}

// ParseAgentsMD parses an AGENTS.md file.
func ParseAgentsMD(content []byte, filePath, repo string) (*AgentsDoc, error) {
	text := string(content)
	lines := strings.Split(text, "\n")

	doc := &AgentsDoc{
		Path: filePath,
		Repo: repo,
	}

	// Extract module from path
	parts := strings.Split(filePath, "/")
	if len(parts) > 1 {
		doc.Module = parts[0]
	}

	// Parse headings and sections
	var currentSection *Section
	var headingStack []string

	headingRe := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	inlineCodeRe := regexp.MustCompile("`([^`]+)`")

	// Track if we just saw an h1 (for description capture)
	justSawH1 := false

	for i, line := range lines {
		if matches := headingRe.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			heading := matches[2]

			// Update heading stack - trim to appropriate level
			for len(headingStack) >= level {
				headingStack = headingStack[:len(headingStack)-1]
			}
			headingStack = append(headingStack, heading)

			// Save previous section
			if currentSection != nil {
				currentSection.EndLine = i - 1
				doc.Sections = append(doc.Sections, *currentSection)
			}

			// Start new section
			currentSection = &Section{
				Heading:     heading,
				HeadingPath: strings.Join(headingStack, " > "),
				Level:       level,
				StartLine:   i + 1,
			}

			// Extract title from first h1
			if level == 1 && doc.Title == "" {
				doc.Title = heading
				justSawH1 = true
			}

			continue
		}

		// Capture description as first non-empty line after h1
		if justSawH1 && strings.TrimSpace(line) != "" {
			doc.Description = strings.TrimSpace(line)
			justSawH1 = false
		}

		// Accumulate content in current section
		if currentSection != nil {
			currentSection.Content += line + "\n"
		}

		// Extract entry points
		if strings.Contains(strings.ToLower(line), "entry point") ||
			(currentSection != nil && strings.Contains(strings.ToLower(currentSection.Heading), "entry")) {
			// Look for code blocks that look like file paths
			for _, match := range inlineCodeRe.FindAllStringSubmatch(line, -1) {
				if isFilePath(match[1]) {
					doc.EntryPoints = append(doc.EntryPoints, match[1])
				}
			}
		}

		// Extract mentioned symbols and files from all inline code
		for _, match := range inlineCodeRe.FindAllStringSubmatch(line, -1) {
			code := match[1]
			if isFilePath(code) {
				doc.MentionedFiles = append(doc.MentionedFiles, code)
			} else if isSymbol(code) {
				doc.MentionedSymbols = append(doc.MentionedSymbols, code)
			}
		}
	}

	// Save last section
	if currentSection != nil {
		currentSection.EndLine = len(lines)
		doc.Sections = append(doc.Sections, *currentSection)
	}

	return doc, nil
}

// ToChunks converts the document to indexable chunks.
func (d *AgentsDoc) ToChunks() []chunk.Chunk {
	var chunks []chunk.Chunk

	for _, section := range d.Sections {
		c := chunk.Chunk{
			Repo:            d.Repo,
			FilePath:        d.Path,
			StartLine:       section.StartLine,
			EndLine:         section.EndLine,
			Type:            chunk.ChunkTypeDoc,
			Kind:            "navigation",
			ModulePath:      d.Module,
			ModuleRoot:      d.Module,
			HeadingPath:     section.HeadingPath,
			Content:         section.Content,
			RetrievalWeight: 1.5, // Boost for navigation docs
		}
		c.ID = chunk.GenerateID(d.Repo, d.Path, section.Heading, section.StartLine)
		chunks = append(chunks, c)
	}

	return chunks
}

func isFilePath(s string) bool {
	return strings.Contains(s, "/") ||
		strings.HasSuffix(s, ".py") ||
		strings.HasSuffix(s, ".js") ||
		strings.HasSuffix(s, ".ts") ||
		strings.HasSuffix(s, ".go") ||
		strings.HasSuffix(s, ".tsx") ||
		strings.HasSuffix(s, ".jsx")
}

func isSymbol(s string) bool {
	// Exclude paths
	if strings.Contains(s, "/") {
		return false
	}

	// PascalCase (e.g., UserService)
	pascalCase := regexp.MustCompile(`^[A-Z][a-zA-Z0-9]+$`)
	if pascalCase.MatchString(s) {
		return true
	}

	// snake_case (e.g., get_user_by_id)
	snakeCase := regexp.MustCompile(`^[a-z_][a-z0-9_]+$`)
	if snakeCase.MatchString(s) {
		return true
	}

	return false
}
