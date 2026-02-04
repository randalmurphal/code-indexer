package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPServerProtocol(t *testing.T) {
	if os.Getenv("VOYAGE_API_KEY") == "" {
		t.Skip("VOYAGE_API_KEY not set")
	}

	// Build MCP server
	projectRoot := getProjectRoot()
	cmd := exec.Command("go", "build", "-o", "bin/code-index-mcp", "./cmd/code-index-mcp")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", output)

	// Start MCP server with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mcpPath := filepath.Join(projectRoot, "bin", "code-index-mcp")
	mcpCmd := exec.CommandContext(ctx, mcpPath, "serve")
	mcpCmd.Env = os.Environ()

	stdin, err := mcpCmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := mcpCmd.StdoutPipe()
	require.NoError(t, err)

	require.NoError(t, mcpCmd.Start())
	defer func() {
		stdin.Close()
		mcpCmd.Process.Kill()
		mcpCmd.Wait()
	}()

	reader := bufio.NewReader(stdout)

	// Test 1: Initialize
	t.Run("initialize", func(t *testing.T) {
		initReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		}

		sendJSONRPC(t, stdin, initReq)
		resp := readJSONRPC(t, reader)

		assert.Equal(t, "2.0", resp["jsonrpc"])
		assert.Equal(t, float64(1), resp["id"])
		assert.Nil(t, resp["error"])

		result, ok := resp["result"].(map[string]interface{})
		require.True(t, ok, "result should be object")
		assert.Equal(t, "2024-11-05", result["protocolVersion"])

		serverInfo, ok := result["serverInfo"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "code-index-mcp", serverInfo["name"])
	})

	// Send initialized notification
	sendJSONRPC(t, stdin, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
	})

	// Test 2: List tools
	t.Run("tools/list", func(t *testing.T) {
		sendJSONRPC(t, stdin, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
		})

		resp := readJSONRPC(t, reader)
		assert.Equal(t, float64(2), resp["id"])
		assert.Nil(t, resp["error"])

		result, ok := resp["result"].(map[string]interface{})
		require.True(t, ok)

		tools, ok := result["tools"].([]interface{})
		require.True(t, ok)
		require.Len(t, tools, 1, "should have 1 tool")

		tool := tools[0].(map[string]interface{})
		assert.Equal(t, "search_code", tool["name"])
		assert.Contains(t, tool["description"], "semantic search")

		inputSchema, ok := tool["inputSchema"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "object", inputSchema["type"])

		props, ok := inputSchema["properties"].(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, props, "query")
		assert.Contains(t, props, "repo")
		assert.Contains(t, props, "limit")
	})

	// Test 3: List resources
	t.Run("resources/list", func(t *testing.T) {
		sendJSONRPC(t, stdin, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "resources/list",
		})

		resp := readJSONRPC(t, reader)
		assert.Equal(t, float64(3), resp["id"])
		assert.Nil(t, resp["error"])

		result, ok := resp["result"].(map[string]interface{})
		require.True(t, ok)

		resources, ok := result["resources"].([]interface{})
		require.True(t, ok)
		require.Len(t, resources, 1, "should have 1 resource")

		resource := resources[0].(map[string]interface{})
		assert.Equal(t, "codeindex://relevant", resource["uri"])
	})

	// Test 4: Call search_code tool (will fail without indexed data, but tests protocol)
	t.Run("tools/call search_code", func(t *testing.T) {
		sendJSONRPC(t, stdin, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      4,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "search_code",
				"arguments": map[string]interface{}{
					"query": "hello world",
					"limit": float64(5),
				},
			},
		})

		resp := readJSONRPC(t, reader)
		assert.Equal(t, float64(4), resp["id"])
		// May have error result if Qdrant not available, but response structure should be valid

		result, hasResult := resp["result"].(map[string]interface{})
		errObj, hasError := resp["error"].(map[string]interface{})

		// Either we get a result or a proper error
		assert.True(t, hasResult || hasError, "should have result or error")

		if hasResult {
			// Check content structure
			content, ok := result["content"].([]interface{})
			if ok && len(content) > 0 {
				firstContent := content[0].(map[string]interface{})
				assert.Equal(t, "text", firstContent["type"])
			}
		}

		if hasError {
			// If error, it should be properly formatted
			assert.Contains(t, errObj, "code")
			assert.Contains(t, errObj, "message")
		}
	})

	// Test 5: Unknown method should return error
	t.Run("unknown method", func(t *testing.T) {
		sendJSONRPC(t, stdin, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      5,
			"method":  "nonexistent/method",
		})

		resp := readJSONRPC(t, reader)
		assert.Equal(t, float64(5), resp["id"])

		errObj, ok := resp["error"].(map[string]interface{})
		require.True(t, ok, "should have error")
		assert.Equal(t, float64(-32601), errObj["code"], "should be method not found error")
	})
}

func sendJSONRPC(t *testing.T, w io.Writer, msg map[string]interface{}) {
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = w.Write(append(data, '\n'))
	require.NoError(t, err)
}

func readJSONRPC(t *testing.T, r *bufio.Reader) map[string]interface{} {
	// Read with timeout
	done := make(chan []byte, 1)
	errCh := make(chan error, 1)

	go func() {
		line, err := r.ReadBytes('\n')
		if err != nil {
			errCh <- err
			return
		}
		done <- line
	}()

	select {
	case line := <-done:
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(line, &resp))
		return resp
	case err := <-errCh:
		require.NoError(t, err, "failed to read response")
		return nil
	case <-time.After(10 * time.Second):
		require.Fail(t, "timeout waiting for response")
		return nil
	}
}
