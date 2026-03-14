package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestOpenAICompatServer creates a test HTTP server that mimics an
// OpenAI-compatible chat completions endpoint.
func newTestOpenAICompatServer(t *testing.T, stream bool, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			// Write SSE chunks.
			words := strings.Fields(content)
			for _, word := range words {
				chunk := map[string]any{
					"choices": []map[string]any{
						{
							"delta":         map[string]string{"content": word + " "},
							"finish_reason": nil,
						},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": content,
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
}

func TestOpenAICompatProvider_Name(t *testing.T) {
	p := NewOpenAICompatProvider("http://localhost:8000/v1", "sqlcoder", "", 15, 0.0)
	if got := p.Name(); got != "openai_compat/sqlcoder" {
		t.Errorf("Name() = %q, want %q", got, "openai_compat/sqlcoder")
	}
}

func TestOpenAICompatProvider_Available(t *testing.T) {
	srv := newTestOpenAICompatServer(t, false, "")
	defer srv.Close()

	p := NewOpenAICompatProvider(srv.URL, "model", "", 5, 0.0)
	if !p.Available(context.Background()) {
		t.Error("Available() = false, want true for reachable server")
	}
}

func TestOpenAICompatProvider_Available_Unreachable(t *testing.T) {
	p := NewOpenAICompatProvider("http://localhost:19999/v1", "model", "", 1, 0.0)
	if p.Available(context.Background()) {
		t.Error("Available() = true, want false for unreachable server")
	}
}

func TestOpenAICompatProvider_GenerateSQL(t *testing.T) {
	expected := "SELECT id, name FROM customers WHERE status = 'active'"
	srv := newTestOpenAICompatServer(t, false, "<sql>"+expected+"</sql>")
	defer srv.Close()

	p := NewOpenAICompatProvider(srv.URL, "sqlcoder", "", 10, 0.0)
	got, err := p.GenerateSQL(context.Background(), SQLGenerationRequest{
		Question:   "Show active customers",
		DBType:     "postgres",
		SchemaContext: "Table: customers (id, name, status)",
	})
	if err != nil {
		t.Fatalf("GenerateSQL() error: %v", err)
	}
	if got != expected {
		t.Errorf("GenerateSQL() = %q, want %q", got, expected)
	}
}

func TestOpenAICompatProvider_ExplainResults(t *testing.T) {
	expected := "Here are your results"
	srv := newTestOpenAICompatServer(t, true, expected)
	defer srv.Close()

	p := NewOpenAICompatProvider(srv.URL, "llama3", "", 10, 0.3)
	var buf strings.Builder
	err := p.ExplainResults(context.Background(), ExplanationRequest{
		Question: "Show active customers",
		SQL:      "SELECT * FROM customers",
	}, &buf)
	if err != nil {
		t.Fatalf("ExplainResults() error: %v", err)
	}
	// The streaming output joins the words back, so just check it contains the content.
	if !strings.Contains(buf.String(), "Here") {
		t.Errorf("ExplainResults() output %q does not contain expected content", buf.String())
	}
}

func TestParseOpenAIStream(t *testing.T) {
	chunks := []string{
		`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		`data: [DONE]`,
	}
	input := strings.Join(chunks, "\n") + "\n"

	var out strings.Builder
	if err := parseOpenAIStream(strings.NewReader(input), &out); err != nil {
		t.Fatalf("parseOpenAIStream error: %v", err)
	}
	if got := out.String(); got != "Hello world" {
		t.Errorf("parseOpenAIStream = %q, want %q", got, "Hello world")
	}
}

func TestParseOpenAIStream_MalformedChunks(t *testing.T) {
	input := "data: not-json\ndata: [DONE]\n"
	var out strings.Builder
	// Should not return error even with malformed chunks — just skip them.
	if err := parseOpenAIStream(strings.NewReader(input), &out); err != nil {
		t.Fatalf("parseOpenAIStream returned unexpected error: %v", err)
	}
}

func TestOpenAICompatProvider_GenerateSQL_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	p := NewOpenAICompatProvider(srv.URL, "model", "", 5, 0.0)
	_, err := p.GenerateSQL(context.Background(), SQLGenerationRequest{Question: "test"})
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestOpenAICompatProvider_APIKey(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "SELECT 1"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	apiKey := "sk-test-key-123"
	p := NewOpenAICompatProvider(srv.URL, "model", apiKey, 5, 0.0)

	// Test Available sets the key.
	p.Available(context.Background())
	if capturedAuth != "Bearer "+apiKey {
		t.Errorf("Available: Authorization header = %q, want %q", capturedAuth, "Bearer "+apiKey)
	}
}

// Ensure the writer error from ExplainResults is propagated.
func TestOpenAICompatProvider_ExplainResults_WriterError(t *testing.T) {
	srv := newTestOpenAICompatServer(t, true, "some content")
	defer srv.Close()

	p := NewOpenAICompatProvider(srv.URL, "model", "", 5, 0.0)
	err := p.ExplainResults(context.Background(), ExplanationRequest{Question: "q"}, errorWriter{})
	if err == nil {
		t.Error("expected error from writer, got nil")
	}
}

// errorWriter implements io.Writer and always returns an error.
// It is used in tests to verify that write errors are propagated correctly.
type errorWriter struct{}

func (errorWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}
