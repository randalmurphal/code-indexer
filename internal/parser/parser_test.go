package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePythonFunction(t *testing.T) {
	code := `
def hello(name: str) -> str:
    """Greet someone by name."""
    return f"Hello, {name}!"
`
	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	symbols, err := p.Parse([]byte(code), "test.py")
	require.NoError(t, err)

	require.Len(t, symbols, 1)
	assert.Equal(t, "hello", symbols[0].Name)
	assert.Equal(t, SymbolFunction, symbols[0].Kind)
	assert.Equal(t, 2, symbols[0].StartLine)
	assert.Equal(t, 4, symbols[0].EndLine)
	assert.Contains(t, symbols[0].Content, "def hello")
	assert.Contains(t, symbols[0].Docstring, "Greet someone")
}

func TestParsePythonClass(t *testing.T) {
	code := `
class User:
    """Represents a user in the system."""

    def __init__(self, name: str):
        self.name = name

    def greet(self) -> str:
        return f"Hello, {self.name}"
`
	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	symbols, err := p.Parse([]byte(code), "test.py")
	require.NoError(t, err)

	// Should have class + 2 methods
	require.Len(t, symbols, 3)

	// Class
	assert.Equal(t, "User", symbols[0].Name)
	assert.Equal(t, SymbolClass, symbols[0].Kind)

	// Methods
	assert.Equal(t, "__init__", symbols[1].Name)
	assert.Equal(t, SymbolMethod, symbols[1].Kind)
	assert.Equal(t, "User", symbols[1].Parent)

	assert.Equal(t, "greet", symbols[2].Name)
	assert.Equal(t, SymbolMethod, symbols[2].Kind)
}

func TestParsePythonNestedFunctions(t *testing.T) {
	code := `
def outer():
    """Outer function."""
    def inner():
        pass
    return inner
`
	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	symbols, err := p.Parse([]byte(code), "test.py")
	require.NoError(t, err)

	// Should have both outer and inner functions
	require.Len(t, symbols, 2)
	assert.Equal(t, "outer", symbols[0].Name)
	assert.Equal(t, "inner", symbols[1].Name)
}

func TestParseJavaScriptFunction(t *testing.T) {
	code := `
function greet(name) {
    return "Hello, " + name;
}
`
	p, err := NewParser(LanguageJavaScript)
	require.NoError(t, err)

	symbols, err := p.Parse([]byte(code), "test.js")
	require.NoError(t, err)

	require.Len(t, symbols, 1)
	assert.Equal(t, "greet", symbols[0].Name)
	assert.Equal(t, SymbolFunction, symbols[0].Kind)
	assert.Equal(t, 2, symbols[0].StartLine)
}

func TestParseJavaScriptClass(t *testing.T) {
	code := `
class User {
    constructor(name) {
        this.name = name;
    }

    greet() {
        return "Hello, " + this.name;
    }
}
`
	p, err := NewParser(LanguageJavaScript)
	require.NoError(t, err)

	symbols, err := p.Parse([]byte(code), "test.js")
	require.NoError(t, err)

	// Should have class + 2 methods
	require.Len(t, symbols, 3)

	assert.Equal(t, "User", symbols[0].Name)
	assert.Equal(t, SymbolClass, symbols[0].Kind)

	assert.Equal(t, "constructor", symbols[1].Name)
	assert.Equal(t, SymbolMethod, symbols[1].Kind)
	assert.Equal(t, "User", symbols[1].Parent)

	assert.Equal(t, "greet", symbols[2].Name)
	assert.Equal(t, SymbolMethod, symbols[2].Kind)
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected Language
		ok       bool
	}{
		{"test.py", LanguagePython, true},
		{"path/to/file.py", LanguagePython, true},
		{"test.js", LanguageJavaScript, true},
		{"test.jsx", LanguageJavaScript, true},
		{"test.ts", LanguageTypeScript, true},
		{"test.tsx", LanguageTypeScript, true},
		{"test.go", "", false},
		{"test.txt", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			lang, ok := DetectLanguage(tc.path)
			assert.Equal(t, tc.ok, ok)
			if ok {
				assert.Equal(t, tc.expected, lang)
			}
		})
	}
}

func TestUnsupportedLanguage(t *testing.T) {
	_, err := NewParser("rust")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported language")
}
