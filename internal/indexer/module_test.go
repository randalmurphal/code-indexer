package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestModuleResolver(t *testing.T) {
	resolver := NewModuleResolver("/repo", &config.RepoConfig{})

	tests := []struct {
		filePath   string
		modulePath string
		moduleRoot string
		submodule  string
	}{
		{"src/utils/helper.py", "src.utils.helper", "src", "utils"},
		{"fisio/fisio/imports/aws.py", "fisio.imports.aws", "fisio", "imports"},
		{"main.py", "main", "main", ""},
		{"lib/auth/service.js", "lib.auth.service", "lib", "auth"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			modulePath, moduleRoot, submodule := resolver.Resolve(tt.filePath)
			assert.Equal(t, tt.modulePath, modulePath)
			assert.Equal(t, tt.moduleRoot, moduleRoot)
			assert.Equal(t, tt.submodule, submodule)
		})
	}
}

func TestModuleResolverCaching(t *testing.T) {
	resolver := NewModuleResolver("/repo", &config.RepoConfig{})

	// First call
	path1, root1, sub1 := resolver.Resolve("src/service.py")

	// Second call should use cache
	path2, root2, sub2 := resolver.Resolve("src/service.py")

	assert.Equal(t, path1, path2)
	assert.Equal(t, root1, root2)
	assert.Equal(t, sub1, sub2)
}

func TestDetectModules(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "module-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create Python package
	pyPkg := filepath.Join(tmpDir, "mypkg")
	os.MkdirAll(pyPkg, 0755)
	os.WriteFile(filepath.Join(pyPkg, "__init__.py"), []byte(""), 0644)

	// Create submodule
	submod := filepath.Join(pyPkg, "utils")
	os.MkdirAll(submod, 0755)
	os.WriteFile(filepath.Join(submod, "__init__.py"), []byte(""), 0644)

	// Create Node package
	nodePkg := filepath.Join(tmpDir, "frontend")
	os.MkdirAll(nodePkg, 0755)
	os.WriteFile(filepath.Join(nodePkg, "package.json"), []byte("{}"), 0644)

	// Detect modules
	modules := DetectModules(tmpDir)

	// Check Python package
	assert.Contains(t, modules, "mypkg")
	assert.Contains(t, modules["mypkg"].Description, "Python")
	assert.Contains(t, modules["mypkg"].Submodules, "utils")

	// Check Node package
	assert.Contains(t, modules, "frontend")
	assert.Contains(t, modules["frontend"].Description, "Node")
}

func TestDetectModulesSkipsHidden(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "module-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create hidden directory
	os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)

	// Create node_modules
	os.MkdirAll(filepath.Join(tmpDir, "node_modules"), 0755)

	// Create venv
	os.MkdirAll(filepath.Join(tmpDir, "venv"), 0755)

	// Create valid package
	validPkg := filepath.Join(tmpDir, "valid")
	os.MkdirAll(validPkg, 0755)
	os.WriteFile(filepath.Join(validPkg, "__init__.py"), []byte(""), 0644)

	modules := DetectModules(tmpDir)

	// Should only have valid package
	assert.NotContains(t, modules, ".git")
	assert.NotContains(t, modules, "node_modules")
	assert.NotContains(t, modules, "venv")
	assert.Contains(t, modules, "valid")
}

func TestDetectSubmodules(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "submodule-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package with submodules
	os.MkdirAll(filepath.Join(tmpDir, "__init__.py"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "__init__.py"), []byte(""), 0644)

	// Python submodule
	sub1 := filepath.Join(tmpDir, "models")
	os.MkdirAll(sub1, 0755)
	os.WriteFile(filepath.Join(sub1, "__init__.py"), []byte(""), 0644)

	// Directory with code files (but no __init__.py)
	sub2 := filepath.Join(tmpDir, "utils")
	os.MkdirAll(sub2, 0755)
	os.WriteFile(filepath.Join(sub2, "helpers.py"), []byte(""), 0644)

	// Private submodule (should be skipped)
	sub3 := filepath.Join(tmpDir, "_private")
	os.MkdirAll(sub3, 0755)
	os.WriteFile(filepath.Join(sub3, "__init__.py"), []byte(""), 0644)

	submodules := detectSubmodules(tmpDir)

	assert.Contains(t, submodules, "models")
	assert.Contains(t, submodules, "utils")
	assert.NotContains(t, submodules, "_private")
}
