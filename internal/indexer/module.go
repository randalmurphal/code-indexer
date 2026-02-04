package indexer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/config"
)

// ModuleResolver resolves file paths to module paths.
type ModuleResolver struct {
	repoPath string
	config   *config.RepoConfig
	cache    map[string]moduleInfo
}

type moduleInfo struct {
	modulePath string
	moduleRoot string
	submodule  string
}

// NewModuleResolver creates a new resolver.
func NewModuleResolver(repoPath string, cfg *config.RepoConfig) *ModuleResolver {
	return &ModuleResolver{
		repoPath: repoPath,
		config:   cfg,
		cache:    make(map[string]moduleInfo),
	}
}

// Resolve converts a file path to module path components.
func (r *ModuleResolver) Resolve(filePath string) (modulePath, moduleRoot, submodule string) {
	if cached, ok := r.cache[filePath]; ok {
		return cached.modulePath, cached.moduleRoot, cached.submodule
	}

	// Get relative path
	relPath := filePath
	if filepath.IsAbs(filePath) {
		relPath, _ = filepath.Rel(r.repoPath, filePath)
	}

	// Remove file extension
	relPath = strings.TrimSuffix(relPath, filepath.Ext(relPath))

	// Convert path separators to dots
	modulePath = strings.ReplaceAll(relPath, string(filepath.Separator), ".")

	// Handle duplicate prefixes (e.g., fisio/fisio -> fisio)
	parts := strings.Split(modulePath, ".")
	if len(parts) >= 2 && parts[0] == parts[1] {
		parts = parts[1:]
		modulePath = strings.Join(parts, ".")
	}

	// Extract root and submodule
	if len(parts) > 0 {
		moduleRoot = parts[0]
	}
	if len(parts) > 1 {
		submodule = parts[1]
	}

	// Cache result
	r.cache[filePath] = moduleInfo{
		modulePath: modulePath,
		moduleRoot: moduleRoot,
		submodule:  submodule,
	}

	return modulePath, moduleRoot, submodule
}

// DetectModules auto-detects module structure from filesystem.
func DetectModules(repoPath string) map[string]config.Module {
	modules := make(map[string]config.Module)

	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return modules
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "venv" || name == "__pycache__" {
			continue
		}

		dirPath := filepath.Join(repoPath, name)

		// Check if it's a Python package
		if _, err := os.Stat(filepath.Join(dirPath, "__init__.py")); err == nil {
			modules[name] = config.Module{
				Description: "Python package: " + name,
				Submodules:  detectSubmodules(dirPath),
			}
			continue
		}

		// Check if it's a JS/TS package
		if _, err := os.Stat(filepath.Join(dirPath, "package.json")); err == nil {
			modules[name] = config.Module{
				Description: "Node package: " + name,
			}
			continue
		}

		// Check if it's a Go module
		if _, err := os.Stat(filepath.Join(dirPath, "go.mod")); err == nil {
			modules[name] = config.Module{
				Description: "Go module: " + name,
			}
			continue
		}

		// Check for nested package (e.g., fisio/fisio)
		nestedPath := filepath.Join(dirPath, name)
		if _, err := os.Stat(filepath.Join(nestedPath, "__init__.py")); err == nil {
			modules[name] = config.Module{
				Description: "Python package: " + name,
				Submodules:  detectSubmodules(nestedPath),
			}
		}
	}

	return modules
}

func detectSubmodules(packagePath string) map[string]string {
	submodules := make(map[string]string)

	entries, err := os.ReadDir(packagePath)
	if err != nil {
		return submodules
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}

		subPath := filepath.Join(packagePath, name)

		// Check for Python submodule
		if _, err := os.Stat(filepath.Join(subPath, "__init__.py")); err == nil {
			submodules[name] = "Submodule: " + name
			continue
		}

		// Check for directory with code files
		hasCode := false
		subEntries, _ := os.ReadDir(subPath)
		for _, e := range subEntries {
			if !e.IsDir() {
				ext := filepath.Ext(e.Name())
				if ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".go" {
					hasCode = true
					break
				}
			}
		}
		if hasCode {
			submodules[name] = "Submodule: " + name
		}
	}

	return submodules
}
