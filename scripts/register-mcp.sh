#!/bin/bash
# scripts/register-mcp.sh
# Register code-index-mcp with Claude Code

set -e

# Get the binary path
BINARY_PATH="${1:-}"
if [ -z "$BINARY_PATH" ]; then
    # Try to find it
    BINARY_PATH=$(which code-index-mcp 2>/dev/null || true)
    if [ -z "$BINARY_PATH" ]; then
        SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
        PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
        BINARY_PATH="$PROJECT_ROOT/bin/code-index-mcp"
    fi
fi

if [ ! -f "$BINARY_PATH" ]; then
    echo "Error: code-index-mcp binary not found at $BINARY_PATH"
    echo ""
    echo "Build it first:"
    echo "  go build -o bin/code-index-mcp ./cmd/code-index-mcp"
    exit 1
fi

# Make path absolute
BINARY_PATH=$(cd "$(dirname "$BINARY_PATH")" && pwd)/$(basename "$BINARY_PATH")

echo "Using binary: $BINARY_PATH"

# Create Claude settings directory
CLAUDE_DIR="$HOME/.claude"
mkdir -p "$CLAUDE_DIR"

SETTINGS_FILE="$CLAUDE_DIR/settings.json"

# Backup existing settings if present
if [ -f "$SETTINGS_FILE" ]; then
    cp "$SETTINGS_FILE" "$SETTINGS_FILE.bak.$(date +%Y%m%d%H%M%S)"
    echo "Backed up existing settings"
fi

# Add MCP server configuration using jq if available
if command -v jq &> /dev/null && [ -f "$SETTINGS_FILE" ]; then
    # Merge into existing settings
    jq --arg path "$BINARY_PATH" '
        .mcpServers = (.mcpServers // {}) |
        .mcpServers["code-index"] = {
            "command": $path,
            "args": ["serve"],
            "env": {
                "VOYAGE_API_KEY": "${VOYAGE_API_KEY}"
            }
        }
    ' "$SETTINGS_FILE" > "$SETTINGS_FILE.tmp" && mv "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
else
    # Create new settings file
    cat > "$SETTINGS_FILE" << EOF
{
  "mcpServers": {
    "code-index": {
      "command": "$BINARY_PATH",
      "args": ["serve"],
      "env": {
        "VOYAGE_API_KEY": "\${VOYAGE_API_KEY}"
      }
    }
  }
}
EOF
fi

echo ""
echo "Registered code-index-mcp with Claude Code"
echo ""
echo "Configuration added to: $SETTINGS_FILE"
echo ""
echo "Prerequisites:"
echo "  1. VOYAGE_API_KEY environment variable must be set"
echo "  2. Qdrant running at localhost:6334"
echo "  3. Redis running at localhost:6379 (optional, for query caching)"
echo ""
echo "Restart Claude Code to load the new MCP server."
echo ""
echo "Available tools after registration:"
echo "  - search_code: Semantic code search across indexed repositories"
