# mcp package

Model Context Protocol types and server implementation.

## Purpose

Implement MCP protocol for Claude Code integration. Provides `search_code` tool and `codeindex://relevant` resource.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Server` | JSON-RPC server | `server.go:15-20` |
| `Tool` | Tool definition | `types.go:8-12` |
| `Resource` | Resource definition | `types.go:20-26` |
| `CallToolResult` | Tool response | `types.go:35-38` |
| `Content` | Response content | `types.go:40-43` |

## Protocol

MCP uses JSON-RPC 2.0 over stdin/stdout:

**Request**:
```json
{"jsonrpc": "2.0", "method": "tools/call", "params": {...}, "id": 1}
```

**Response**:
```json
{"jsonrpc": "2.0", "result": {...}, "id": 1}
```

## Tool Schema

`search_code` tool defined in `search/handler.go:92-126`:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Natural language query |
| `repo` | string | No | Repository filter |
| `module` | string | No | Module path filter |
| `include_tests` | string | No | include/exclude/only |
| `limit` | number | No | Max results (default: 10) |
| `cursor` | string | No | Pagination cursor |

## Server Lifecycle

```go
server := mcp.NewServer(handler, logger)
server.Run(ctx)  // Blocks, reads stdin, writes stdout
```

## Handler Interface

Handlers implement:
```go
type Handler interface {
    ListTools() []Tool
    CallTool(ctx, name, args) (*CallToolResult, error)
    ListResources() []Resource
    ReadResource(ctx, uri) (*ReadResourceResult, error)
}
```

## Claude Code Registration

In `~/.claude/mcp.json`:
```json
{
  "code-index": {
    "command": "/path/to/code-index-mcp",
    "env": {"VOYAGE_API_KEY": "..."}
  }
}
```

## Gotchas

1. **Stdio only**: MCP uses stdin/stdout, no HTTP
2. **Single handler**: Server wraps one handler instance
3. **Graceful shutdown**: Context cancellation stops server
4. **Error format**: Errors returned in `CallToolResult.IsError`
