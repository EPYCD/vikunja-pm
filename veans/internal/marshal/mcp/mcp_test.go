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

package mcp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/veans/internal/marshal/mcp"
)

const timeout = 5 * time.Second

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type callResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func okHandler(_ context.Context, _ json.RawMessage) (mcp.Result, error) {
	return mcp.Result{}, nil
}

// echoTool returns its "text" argument verbatim.
func echoTool() mcp.Tool {
	return mcp.Tool{
		Name:        "echo",
		Description: "Echo text",
		InputSchema: mcp.Schema(map[string]any{"text": mcp.Prop("string", "What to echo")}, "text"),
		Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return mcp.Result{}, err
			}
			return mcp.Result{Text: in.Text}, nil
		},
	}
}

func newServer(t *testing.T, tools ...mcp.Tool) *mcp.Server {
	t.Helper()
	s := mcp.NewServer("test-server", "1.2.3")
	for _, tool := range tools {
		s.Register(tool)
	}
	return s
}

// serve runs raw input through Serve to EOF and returns the decoded replies.
func serve(t *testing.T, s *mcp.Server, input string) []reply {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	return parseReplies(t, out.Bytes())
}

// run sends the messages one per line.
func run(t *testing.T, s *mcp.Server, messages ...string) []reply {
	t.Helper()
	return serve(t, s, strings.Join(messages, "\n")+"\n")
}

// one sends a single request and expects exactly one reply.
func one(t *testing.T, s *mcp.Server, message string) reply {
	t.Helper()
	replies := run(t, s, message)
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1: %+v", len(replies), replies)
	}
	return replies[0]
}

func parseReplies(t *testing.T, out []byte) []reply {
	t.Helper()
	if len(out) > 0 && out[len(out)-1] != '\n' {
		t.Fatalf("output does not end with a newline: %q", out)
	}
	var replies []reply
	for _, line := range bytes.Split(out, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var r reply
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("reply is not valid JSON: %v\n%s", err, line)
		}
		if r.JSONRPC != "2.0" {
			t.Fatalf("reply jsonrpc = %q, want 2.0", r.JSONRPC)
		}
		if (r.Result == nil) == (r.Error == nil) {
			t.Fatalf("reply must carry exactly one of result/error: %s", line)
		}
		replies = append(replies, r)
	}
	return replies
}

func decodeResult(t *testing.T, r reply, v any) {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("unexpected error reply: %+v", *r.Error)
	}
	if err := json.Unmarshal(r.Result, v); err != nil {
		t.Fatalf("decode result %s: %v", r.Result, err)
	}
}

func expectError(t *testing.T, r reply, code int) {
	t.Helper()
	if r.Error == nil {
		t.Fatalf("got result %s, want error %d", r.Result, code)
	}
	if r.Error.Code != code {
		t.Fatalf("error code = %d (%q), want %d", r.Error.Code, r.Error.Message, code)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestInitialize_EchoesSupportedVersion(t *testing.T) {
	for _, v := range []string{"2025-06-18", "2025-03-26", "2024-11-05"} {
		r := one(t, newServer(t), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+v+
			`","capabilities":{},"clientInfo":{"name":"c","version":"0"}}}`)
		var res struct {
			ProtocolVersion string                     `json:"protocolVersion"`
			Capabilities    map[string]json.RawMessage `json:"capabilities"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		}
		decodeResult(t, r, &res)
		if res.ProtocolVersion != v {
			t.Errorf("protocolVersion = %q, want %q echoed", res.ProtocolVersion, v)
		}
		if tools, ok := res.Capabilities["tools"]; !ok || string(tools) != "{}" {
			t.Errorf("capabilities = %v, want {\"tools\":{}}", res.Capabilities)
		}
		if res.ServerInfo.Name != "test-server" || res.ServerInfo.Version != "1.2.3" {
			t.Errorf("serverInfo = %+v", res.ServerInfo)
		}
	}
}

func TestInitialize_UnknownVersionFallsBackToLatest(t *testing.T) {
	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2099-01-01"}}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":42}}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
	} {
		var res struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		decodeResult(t, one(t, newServer(t), msg), &res)
		if res.ProtocolVersion != "2025-06-18" {
			t.Errorf("%s: protocolVersion = %q, want 2025-06-18", msg, res.ProtocolVersion)
		}
	}
}

func TestNotifications_GetNoReply(t *testing.T) {
	replies := run(t, newServer(t),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`,
		`{"jsonrpc":"2.0","method":"ping"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	if len(replies) != 1 || string(replies[0].ID) != "2" {
		t.Fatalf("replies = %+v, want only the reply to id 2", replies)
	}
}

func TestPing(t *testing.T) {
	r := one(t, newServer(t), `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if r.Error != nil || string(r.Result) != "{}" {
		t.Fatalf("ping reply = %+v, want result {}", r)
	}
}

func TestToolsList(t *testing.T) {
	noop := mcp.Tool{Name: "noop", Handler: okHandler}
	r := one(t, newServer(t, echoTool(), noop), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	want := `{"tools":[` +
		`{"name":"echo","description":"Echo text","inputSchema":{"properties":{"text":{"description":"What to echo","type":"string"}},"required":["text"],"type":"object"}},` +
		`{"name":"noop","inputSchema":{"properties":{},"type":"object"}}` +
		`]}`
	if got := string(r.Result); got != want {
		t.Fatalf("tools/list result:\n got %s\nwant %s", got, want)
	}
}

func TestToolsCall_HappyPath(t *testing.T) {
	r := one(t, newServer(t, echoTool()),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hello"}}}`)
	var res callResult
	decodeResult(t, r, &res)
	if res.IsError {
		t.Fatal("isError = true, want false")
	}
	if len(res.Content) != 1 || res.Content[0].Type != "text" || res.Content[0].Text != "hello" {
		t.Fatalf("content = %+v, want one text item \"hello\"", res.Content)
	}
}

func TestToolsCall_HandlerReceivesServeContext(t *testing.T) {
	type key struct{}
	var seen any
	peek := mcp.Tool{Name: "peek", Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		seen = ctx.Value(key{})
		return mcp.Result{}, nil
	}}
	ctx := context.WithValue(context.Background(), key{}, "marker")
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"peek"}}` + "\n")
	if err := newServer(t, peek).Serve(ctx, in, io.Discard); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if seen != "marker" {
		t.Fatalf("handler ctx value = %v, want marker", seen)
	}
}

func TestToolsCall_MissingArgumentsBecomeEmptyObject(t *testing.T) {
	var got string
	peek := mcp.Tool{Name: "peek", Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
		got = string(args)
		return mcp.Result{}, nil
	}}
	for _, params := range []string{`{"name":"peek"}`, `{"name":"peek","arguments":null}`} {
		got = ""
		one(t, newServer(t, peek), `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+params+`}`)
		if got != "{}" {
			t.Errorf("params %s: handler got %q, want {}", params, got)
		}
	}
}

func TestToolsCall_UnknownTool(t *testing.T) {
	r := one(t, newServer(t, echoTool()),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	expectError(t, r, -32602)
	if !strings.Contains(r.Error.Message, "nope") {
		t.Fatalf("message %q should name the tool", r.Error.Message)
	}
}

func TestToolsCall_InvalidParams(t *testing.T) {
	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"echo"}`,
	} {
		expectError(t, one(t, newServer(t, echoTool()), msg), -32602)
	}
}

func TestToolsCall_HandlerErrorBecomesErrorResult(t *testing.T) {
	fail := mcp.Tool{Name: "fail", Handler: func(_ context.Context, _ json.RawMessage) (mcp.Result, error) {
		return mcp.Result{Text: "ignored"}, errors.New("task 42 not found")
	}}
	r := one(t, newServer(t, fail), `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fail"}}`)
	var res callResult
	decodeResult(t, r, &res)
	if !res.IsError || len(res.Content) != 1 || res.Content[0].Text != "task 42 not found" {
		t.Fatalf("result = %+v, want isError with the error text", res)
	}
}

func TestToolsCall_HandlerIsErrorResultIsForwarded(t *testing.T) {
	soft := mcp.Tool{Name: "soft", Handler: func(_ context.Context, _ json.RawMessage) (mcp.Result, error) {
		return mcp.Result{Text: "nothing to do", IsError: true}, nil
	}}
	r := one(t, newServer(t, soft), `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"soft"}}`)
	var res callResult
	decodeResult(t, r, &res)
	if !res.IsError || res.Content[0].Text != "nothing to do" {
		t.Fatalf("result = %+v", res)
	}
}

func TestToolsCall_HandlerPanicIsContained(t *testing.T) {
	boom := mcp.Tool{Name: "boom", Handler: func(_ context.Context, _ json.RawMessage) (mcp.Result, error) {
		panic("kaboom")
	}}
	replies := run(t, newServer(t, boom),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"boom"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	if len(replies) != 2 {
		t.Fatalf("got %d replies, want 2 (server must survive the panic)", len(replies))
	}
	var res callResult
	decodeResult(t, replies[0], &res)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "kaboom") {
		t.Fatalf("result = %+v, want isError mentioning kaboom", res)
	}
	if string(replies[1].ID) != "2" {
		t.Fatalf("second reply id = %s, want 2", replies[1].ID)
	}
}

func TestMalformedLine_ParseErrorThenKeepsServing(t *testing.T) {
	replies := run(t, newServer(t), `{"jsonrpc":"2.0","id":1,`, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if len(replies) != 2 {
		t.Fatalf("got %d replies, want 2", len(replies))
	}
	expectError(t, replies[0], -32700)
	if string(replies[0].ID) != "null" {
		t.Fatalf("parse error id = %s, want null", replies[0].ID)
	}
	if string(replies[1].ID) != "2" || string(replies[1].Result) != "{}" {
		t.Fatalf("ping after parse error = %+v", replies[1])
	}
}

func TestInvalidRequest(t *testing.T) {
	for msg, wantID := range map[string]string{
		`{"jsonrpc":"2.0","id":3}`: "3",
		`[]`:                       "null",
		`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`: "null",
		`"ping"`: "null",
		`{"jsonrpc":"1.0","id":4,"method":"ping"}`: "4",
		`{"jsonrpc":"2.0","id":5,"method":7}`:      "5",
	} {
		r := one(t, newServer(t), msg)
		expectError(t, r, -32600)
		if string(r.ID) != wantID {
			t.Errorf("%s: id = %s, want %s", msg, r.ID, wantID)
		}
	}
}

func TestClientResponseIsIgnored(t *testing.T) {
	replies := run(t, newServer(t),
		`{"jsonrpc":"2.0","id":9,"result":{}}`,
		`{"jsonrpc":"2.0","id":10,"error":{"code":-1,"message":"x"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	if len(replies) != 1 || string(replies[0].ID) != "2" {
		t.Fatalf("replies = %+v, want only the ping reply", replies)
	}
}

func TestUnknownMethod(t *testing.T) {
	r := one(t, newServer(t), `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	expectError(t, r, -32601)
	if !strings.Contains(r.Error.Message, "resources/list") {
		t.Fatalf("message %q should name the method", r.Error.Message)
	}
}

func TestIDsEchoedWithSameJSONType(t *testing.T) {
	for _, id := range []string{`"abc"`, `"7"`, `7`, `0`, `12345678901234567890`} {
		r := one(t, newServer(t), `{"jsonrpc":"2.0","id":`+id+`,"method":"ping"}`)
		if string(r.ID) != id {
			t.Errorf("id %s echoed as %s", id, r.ID)
		}
	}
}

func TestLargeArgumentRoundTrips(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	args := mustJSON(t, map[string]string{"text": big})
	r := one(t, newServer(t, echoTool()),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":%s}}`, args))
	var res callResult
	decodeResult(t, r, &res)
	if res.IsError || len(res.Content) != 1 {
		t.Fatalf("isError = %v, %d content items", res.IsError, len(res.Content))
	}
	if got := res.Content[0].Text; got != big {
		t.Fatalf("echoed %d bytes, want %d identical bytes", len(got), len(big))
	}
}

func TestBlankLinesAndCRLFAreTolerated(t *testing.T) {
	replies := serve(t, newServer(t), "\r\n\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\r\n   \n")
	if len(replies) != 1 || string(replies[0].ID) != "1" {
		t.Fatalf("replies = %+v, want one ping reply", replies)
	}
}

func TestServe_ReturnsWhenContextCancelled(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- newServer(t).Serve(ctx, pr, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve returned %v, want context.Canceled", err)
		}
	case <-time.After(timeout):
		t.Fatal("Serve did not return after cancel")
	}
}

func TestServe_CancelReachesRunningHandler(t *testing.T) {
	started := make(chan struct{})
	block := mcp.Tool{Name: "block", Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		close(started)
		<-ctx.Done()
		return mcp.Result{Text: "released"}, nil
	}}
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- newServer(t, block).Serve(ctx, pr, &out) }()

	if _, err := io.WriteString(pw, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block"}}`+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-started:
	case <-time.After(timeout):
		t.Fatal("handler never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve returned %v, want context.Canceled", err)
		}
	case <-time.After(timeout):
		t.Fatal("Serve did not return after cancel")
	}
	replies := parseReplies(t, out.Bytes())
	var res callResult
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want the handler's reply", len(replies))
	}
	decodeResult(t, replies[0], &res)
	if res.Content[0].Text != "released" {
		t.Fatalf("result = %+v", res)
	}
}

var errWrite = errors.New("write failed")

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestServe_WriteErrorIsReturned(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	err := newServer(t).Serve(context.Background(), in, failWriter{})
	if !errors.Is(err, errWrite) {
		t.Fatalf("Serve returned %v, want errWrite", err)
	}
}

func TestServeStdio_FlushesEachMessage(t *testing.T) {
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origIn, origOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	t.Cleanup(func() {
		os.Stdin, os.Stdout = origIn, origOut
		for _, f := range []*os.File{inR, inW, outR, outW} {
			_ = f.Close()
		}
	})

	done := make(chan error, 1)
	go func() { done <- newServer(t).ServeStdio(context.Background()) }()

	if _, err := io.WriteString(inW, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	lineCh := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(outR).ReadString('\n')
		lineCh <- line
	}()
	select {
	case line := <-lineCh:
		replies := parseReplies(t, []byte(line))
		if len(replies) != 1 || string(replies[0].ID) != "1" || string(replies[0].Result) != "{}" {
			t.Fatalf("reply = %+v", replies)
		}
	case <-time.After(timeout):
		t.Fatal("no reply on stdout: the response was not flushed")
	}

	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeStdio returned %v, want nil at EOF", err)
		}
	case <-time.After(timeout):
		t.Fatal("ServeStdio did not return at EOF")
	}
}

func TestRegister_RejectsProgrammingErrors(t *testing.T) {
	cases := map[string]func(*mcp.Server){
		"empty name":  func(s *mcp.Server) { s.Register(mcp.Tool{Handler: okHandler}) },
		"nil handler": func(s *mcp.Server) { s.Register(mcp.Tool{Name: "x"}) },
		"duplicate": func(s *mcp.Server) {
			s.Register(mcp.Tool{Name: "x", Handler: okHandler})
			s.Register(mcp.Tool{Name: "x", Handler: okHandler})
		},
		"unencodable schema": func(s *mcp.Server) {
			s.Register(mcp.Tool{Name: "x", InputSchema: map[string]any{"type": make(chan int)}, Handler: okHandler})
		},
	}
	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Register did not panic")
				}
			}()
			register(mcp.NewServer("t", "0"))
		})
	}
}

func TestSchemaAndProp(t *testing.T) {
	got := mustJSON(t, mcp.Schema(map[string]any{
		"task": mcp.Prop("string", "The task"),
		"n":    mcp.Prop("integer", ""),
	}, "task"))
	want := `{"properties":{"n":{"type":"integer"},"task":{"description":"The task","type":"string"}},"required":["task"],"type":"object"}`
	if got != want {
		t.Errorf("Schema:\n got %s\nwant %s", got, want)
	}
	if got := mustJSON(t, mcp.Schema(nil)); got != `{"properties":{},"type":"object"}` {
		t.Errorf("Schema(nil) = %s", got)
	}
}
