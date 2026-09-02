// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package mcp is a minimal Model Context Protocol server for the stdio
// transport: JSON-RPC 2.0, one message per line, tools only (no resources,
// prompts or sampling). Stdlib only.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// latestProtocolVersion is offered to clients asking for a version we don't speak.
const latestProtocolVersion = "2025-06-18"

var supportedProtocolVersions = map[string]bool{
	latestProtocolVersion: true,
	"2025-03-26":          true,
	"2024-11-05":          true,
}

// maxLineBytes bounds one message; bufio's 64 KiB default is too small for real tool arguments.
const maxLineBytes = 10 << 20

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Tool describes one callable tool.
type Tool struct {
	Name        string
	Description string
	// InputSchema is a JSON Schema object (draft 2020-12 subset) as a generic map, e.g.
	// {"type":"object","properties":{"task":{"type":"string"}},"required":["task"]}.
	// Nil is advertised as an object with no properties.
	InputSchema map[string]any
	// Handler receives the raw JSON arguments ("{}" when the client sent none) and returns
	// text content plus whether it is an error result (isError:true). A non-nil error is
	// reported the same way — protocol errors are reserved for malformed requests.
	Handler func(ctx context.Context, args json.RawMessage) (Result, error)
}

// Result is the outcome of a tool call.
type Result struct {
	Text    string // becomes content[0] of type "text"
	IsError bool
}

// Server speaks MCP over a line-delimited JSON-RPC stream. Register tools,
// then Serve; Register is not safe to call once Serve is running.
type Server struct {
	// Logger receives diagnostics. It defaults to io.Discard and must never
	// point at stdout, which is the protocol channel.
	Logger *log.Logger

	name    string
	version string
	tools   []Tool

	writeMu sync.Mutex
}

// NewServer returns a Server that identifies itself with name and version
// in the initialize handshake.
func NewServer(name, version string) *Server {
	return &Server{
		Logger:  log.New(io.Discard, "", 0),
		name:    name,
		version: version,
	}
}

// Register adds a tool. It panics on an empty name, a nil handler, a
// duplicate name or a schema json.Marshal rejects — programming errors
// better caught at startup than mid-session.
func (s *Server) Register(t Tool) {
	if t.Name == "" {
		panic("mcp: Register: tool name is empty")
	}
	if t.Handler == nil {
		panic("mcp: Register: tool " + t.Name + " has no handler")
	}
	if _, dup := s.find(t.Name); dup {
		panic("mcp: Register: tool " + t.Name + " registered twice")
	}
	if t.InputSchema == nil {
		t.InputSchema = Schema(nil)
	}
	if _, err := json.Marshal(t.InputSchema); err != nil {
		panic("mcp: Register: tool " + t.Name + " has an unencodable input schema: " + err.Error())
	}
	s.tools = append(s.tools, t)
}

func (s *Server) find(name string) (Tool, bool) {
	for _, t := range s.tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Serve reads newline-delimited JSON-RPC messages from r until EOF or ctx is
// done and writes one response per line to w. Requests are handled one at a
// time, in order. After cancellation the goroutine reading r stays blocked
// until r yields, so the caller owns closing r.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	// Own cancel so the reader goroutine is released on every return path,
	// not only the caller's cancellation.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lines := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
		for sc.Scan() {
			select {
			case lines <- bytes.Clone(sc.Bytes()): // the scanner reuses its buffer
			case <-ctx.Done():
				return
			}
		}
		readErr <- sc.Err()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			if err != nil {
				return fmt.Errorf("mcp: reading request: %w", err)
			}
			return nil
		case line := <-lines:
			if err := s.handleLine(ctx, line, w); err != nil {
				return err
			}
		}
	}
}

// ServeStdio is Serve on the process's stdin/stdout, flushing after every
// message so a client blocked on the pipe never waits for a full buffer.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.Serve(ctx, os.Stdin, &flushWriter{bw: bufio.NewWriter(os.Stdout)})
}

type flushWriter struct{ bw *bufio.Writer }

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.bw.Write(p)
	if err != nil {
		return n, err
	}
	return n, f.bw.Flush()
}

func (s *Server) handleLine(ctx context.Context, line []byte, w io.Writer) error {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		s.logf("mcp: rejecting message: %v", err)
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return s.write(w, errorResponse(nil, codeParseError, "Parse error"))
		}
		// Valid JSON of the wrong shape (an array, a string, a numeric method):
		// Unmarshal still fills the fields it could, so the id may be usable.
		return s.write(w, errorResponse(req.ID, codeInvalidRequest, "Invalid Request"))
	}
	if resp := s.dispatch(ctx, &req); resp != nil {
		return s.write(w, resp)
	}
	return nil
}

// dispatch returns the reply for req, or nil when none is due.
func (s *Server) dispatch(ctx context.Context, req *request) *response {
	if req.Method == "" {
		// A reply to a server-initiated request; we send none, so drop it rather
		// than answer with an error the client would have to swallow.
		if len(req.Result) > 0 || len(req.Error) > 0 {
			return nil
		}
		return errorResponse(req.ID, codeInvalidRequest, "Invalid Request: missing method")
	}
	if req.JSONRPC != "2.0" {
		return errorResponse(req.ID, codeInvalidRequest, `Invalid Request: jsonrpc must be "2.0"`)
	}
	if len(req.ID) == 0 {
		// A notification (initialized, cancelled, ...): nothing to act on, never a reply.
		return nil
	}
	switch req.Method {
	case "initialize":
		return okResponse(req.ID, s.initialize(req.Params))
	case "ping":
		return okResponse(req.ID, struct{}{})
	case "tools/list":
		return okResponse(req.ID, s.listTools())
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return errorResponse(req.ID, codeMethodNotFound, "Method not found: "+req.Method)
	}
}

func (s *Server) initialize(params json.RawMessage) initializeResult {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			s.logf("mcp: initialize: ignoring malformed params: %v", err)
		}
	}
	version := p.ProtocolVersion
	if !supportedProtocolVersions[version] {
		version = latestProtocolVersion
	}
	return initializeResult{
		ProtocolVersion: version,
		ServerInfo:      serverInfo{Name: s.name, Version: s.version},
	}
}

func (s *Server) listTools() listToolsResult {
	tools := make([]toolDescriptor, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, toolDescriptor{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return listToolsResult{Tools: tools}
}

func (s *Server) callTool(ctx context.Context, req *request) *response {
	if len(req.Params) == 0 {
		return errorResponse(req.ID, codeInvalidParams, "Invalid params: missing params")
	}
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, codeInvalidParams, "Invalid params: "+err.Error())
	}
	if p.Name == "" {
		return errorResponse(req.ID, codeInvalidParams, "Invalid params: name is required")
	}
	t, ok := s.find(p.Name)
	if !ok {
		return errorResponse(req.ID, codeInvalidParams, "Unknown tool: "+p.Name)
	}
	args := p.Arguments
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	res := s.invoke(ctx, t, args)
	return okResponse(req.ID, callToolResult{
		Content: []textContent{{Type: "text", Text: res.Text}},
		IsError: res.IsError,
	})
}

// invoke folds a handler's returned error or panic into an isError result so
// one misbehaving tool cannot take the whole session down.
func (s *Server) invoke(ctx context.Context, t Tool, args json.RawMessage) (res Result) {
	defer func() {
		if r := recover(); r != nil {
			s.logf("mcp: tool %s panicked: %v", t.Name, r)
			res = Result{Text: fmt.Sprintf("tool %s panicked: %v", t.Name, r), IsError: true}
		}
	}()
	out, err := t.Handler(ctx, args)
	if err != nil {
		return Result{Text: err.Error(), IsError: true}
	}
	return out
}

func (s *Server) write(w io.Writer, resp *response) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("mcp: encoding response: %w", err)
	}
	b = append(b, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("mcp: writing response: %w", err)
	}
	return nil
}

func (s *Server) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

// Schema builds the input schema for a tool: an object with the given
// properties (see Prop) and the names of the required ones.
func Schema(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// Prop builds one property schema of the given JSON Schema type ("string",
// "integer", "boolean", ...) with an optional description.
func Prop(typ, desc string) map[string]any {
	p := map[string]any{"type": typ}
	if desc != "" {
		p["description"] = desc
	}
	return p
}

// Wire types. Field names follow the MCP schema, hence camelCase tags.

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	// Only present on a client's reply to a server-initiated request; lets
	// dispatch tell one from a malformed request.
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func okResponse(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type capabilities struct {
	Tools struct{} `json:"tools"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

type listToolsResult struct {
	Tools []toolDescriptor `json:"tools"`
}

type callToolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
