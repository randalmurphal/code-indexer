#!/bin/bash
# scripts/install-hooks.sh
# Install Claude Code hooks for a repository

set -e

REPO_PATH="${1:-.}"
REPO_PATH=$(cd "$REPO_PATH" && pwd)

# Verify it's a git repo
if [ ! -d "$REPO_PATH/.git" ]; then
    echo "Error: $REPO_PATH is not a git repository"
    exit 1
fi

# Check that code-indexer is available
if ! command -v code-indexer &> /dev/null; then
    echo "Warning: code-indexer not found in PATH"
    echo "Make sure to build and install it first:"
    echo "  go build -o ~/go/bin/code-indexer ./cmd/code-indexer"
fi

# Create .claude directory if needed
CLAUDE_DIR="$REPO_PATH/.claude"
mkdir -p "$CLAUDE_DIR"

# Create settings.json with hooks
cat > "$CLAUDE_DIR/settings.json" << 'EOF'
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read",
        "hooks": [{
          "type": "command",
          "command": "code-indexer suggest-context \"$CLAUDE_FILE_PATH\" 2>&1 >&2 || true"
        }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [{
          "type": "command",
          "command": "code-indexer invalidate-file \"$CLAUDE_FILE_PATH\" 2>&1 >&2 || true"
        }]
      }
    ]
  }
}
EOF

echo "Installed Claude Code hooks to $CLAUDE_DIR/settings.json"
echo ""
echo "Hooks configured:"
echo "  - PreToolUse/Read: Suggests related files from the index"
echo "  - PostToolUse/Write|Edit: Invalidates cache for changed files"
echo ""
echo "Prerequisites:"
echo "  1. code-indexer binary in PATH"
echo "  2. VOYAGE_API_KEY environment variable set"
echo "  3. Qdrant running at localhost:6334"
echo "  4. Redis running at localhost:6379 (optional, for caching)"
echo ""
echo "To index this repository:"
echo "  code-indexer index $REPO_PATH"
