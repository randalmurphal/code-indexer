package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// Handler defines the interface for MCP request handlers.
// Implement this interface to provide tool and resource functionality.
type Handler interface {
	// ListTools returns the available tools.
	ListTools() []Tool

	// CallTool executes a tool and returns the result.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error)

	// ListResources returns the available resources.
	ListResources() []Resource

	// ReadResource reads a resource by URI.
	ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error)
}

// Server implements an MCP server with stdio transport.
type Server struct {
	name    string
	version string
	handler Handler
	logger  *slog.Logger

	reader io.Reader
	writer io.Writer
	mu     sync.Mutex
}

// NewServer creates a new MCP server.
func NewServer(name, version string, handler Handler, logger *slog.Logger) *Server {
	return &Server{
		name:    name,
		version: version,
		handler: handler,
		logger:  logger,
	}
}

// Run starts the server, reading from stdin and writing to stdout.
func (s *Server) Run(ctx context.Context, reader io.Reader, writer io.Writer) error {
	s.reader = reader
	s.writer = writer

	scanner := bufio.NewScanner(reader)
	// Increase buffer size for large messages
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	s.logger.Info("MCP server started", "name", s.name, "version", s.version)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			s.logger.Info("server shutting down")
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		s.logger.Debug("received request", "raw", string(line))

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Error("failed to parse request", "error", err)
			s.sendError(nil, ErrCodeParse, "Parse error", err.Error())
			continue
		}

		response := s.handleRequest(ctx, &req)
		if response != nil {
			s.sendResponse(response)
		}
	}

	if err := scanner.Err(); err != nil {
		s.logger.Error("scanner error", "error", err)
		return err
	}

	return nil
}

func (s *Server) handleRequest(ctx context.Context, req *Request) *Response {
	s.logger.Debug("handling request", "method", req.Method, "id", req.ID)

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)

	case "initialized":
		// Notification, no response needed
		s.logger.Info("client initialized")
		return nil

	case "tools/list":
		return s.handleListTools(req)

	case "tools/call":
		return s.handleCallTool(ctx, req)

	case "resources/list":
		return s.handleListResources(req)

	case "resources/read":
		return s.handleReadResource(ctx, req)

	case "ping":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}

	default:
		s.logger.Warn("unknown method", "method", req.Method)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    ErrCodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	var params InitializeParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.logger.Error("failed to parse initialize params", "error", err)
		}
	}

	s.logger.Info("initializing",
		"client", params.ClientInfo.Name,
		"clientVersion", params.ClientInfo.Version,
		"protocolVersion", params.ProtocolVersion)

	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    s.name,
			Version: s.version,
		},
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleListTools(req *Request) *Response {
	tools := s.handler.ListTools()

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ListToolsResult{Tools: tools},
	}
}

func (s *Server) handleCallTool(ctx context.Context, req *Request) *Response {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    ErrCodeInvalidParams,
				Message: "Invalid params",
				Data:    err.Error(),
			},
		}
	}

	s.logger.Info("calling tool", "name", params.Name)

	result, err := s.handler.CallTool(ctx, params.Name, params.Arguments)
	if err != nil {
		s.logger.Error("tool call failed", "name", params.Name, "error", err)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: CallToolResult{
				Content: []Content{{Type: "text", Text: err.Error()}},
				IsError: true,
			},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleListResources(req *Request) *Response {
	resources := s.handler.ListResources()

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ListResourcesResult{Resources: resources},
	}
}

func (s *Server) handleReadResource(ctx context.Context, req *Request) *Response {
	var params ReadResourceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    ErrCodeInvalidParams,
				Message: "Invalid params",
				Data:    err.Error(),
			},
		}
	}

	s.logger.Info("reading resource", "uri", params.URI)

	result, err := s.handler.ReadResource(ctx, params.URI)
	if err != nil {
		s.logger.Error("resource read failed", "uri", params.URI, "error", err)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    ErrCodeInternal,
				Message: "Resource read failed",
				Data:    err.Error(),
			},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) sendResponse(resp *Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		return
	}

	s.logger.Debug("sending response", "raw", string(data))

	// Write response followed by newline
	_, err = fmt.Fprintf(s.writer, "%s\n", data)
	if err != nil {
		s.logger.Error("failed to write response", "error", err)
	}
}

func (s *Server) sendError(id interface{}, code int, message, data string) {
	resp := &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.sendResponse(resp)
}
