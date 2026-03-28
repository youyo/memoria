package embedding_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/embedding"
)

// mockRoundTripper はテスト用の HTTP RoundTripper。
type mockRoundTripper struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.fn(req)
}

// responseBody は JSON オブジェクトを http.Response.Body として返す。
func responseBody(v any) io.ReadCloser {
	b, _ := json.Marshal(v)
	return io.NopCloser(bytes.NewReader(b))
}

// newHealthyClient は /health に ok を返す mock client を作る。
func newHealthyClient() *embedding.Client {
	rt := &mockRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/health":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       responseBody(embedding.HealthResponse{Status: "ok", Model: "test-model", Dimensions: 256, Device: "cpu"}),
			}, nil
		case "/embed":
			var embedReq embedding.EmbedRequest
			json.NewDecoder(req.Body).Decode(&embedReq)
			vecs := make([][]float32, len(embedReq.Texts))
			for i := range vecs {
				vecs[i] = make([]float32, 256)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       responseBody(embedding.EmbedResponse{Embeddings: vecs, Model: "test-model", Dimensions: 256}),
			}, nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}
	}}
	return embedding.NewWithHTTPClient("http://localhost", &http.Client{Transport: rt})
}

// newSlowClient は delay 後にレスポンスを返す mock client を作る。
func newSlowClient(delay time.Duration) *embedding.Client {
	rt := &mockRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       responseBody(embedding.HealthResponse{Status: "ok", Model: "test-model", Dimensions: 256, Device: "cpu"}),
		}, nil
	}}
	return embedding.NewWithHTTPClient("http://localhost", &http.Client{Transport: rt})
}

// newErrorClient は指定のステータスコードを返す mock client を作る。
func newErrorClient(statusCode int) *embedding.Client {
	rt := &mockRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	}}
	return embedding.NewWithHTTPClient("http://localhost", &http.Client{Transport: rt})
}

// --- tests ---

func TestHealth_Success(t *testing.T) {
	client := newHealthyClient()
	resp, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Dimensions != 256 {
		t.Errorf("expected 256 dimensions, got %d", resp.Dimensions)
	}
}

func TestHealth_WorkerNotRunning(t *testing.T) {
	// New() で UDS を指定するが接続できない場合のテスト
	client := embedding.New("/nonexistent/path.sock")
	_, err := client.Health(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestHealth_ContextTimeout(t *testing.T) {
	client := newSlowClient(200 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.Health(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestEmbed_Success(t *testing.T) {
	client := newHealthyClient()
	vecs, err := client.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 2 {
		t.Errorf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 256 {
		t.Errorf("expected 256 dimensions, got %d", len(vecs[0]))
	}
}

func TestEmbed_EmptyTexts(t *testing.T) {
	client := newHealthyClient()
	vecs, err := client.Embed(context.Background(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("expected 0 vectors, got %d", len(vecs))
	}
}

func TestEmbed_ServerError(t *testing.T) {
	client := newErrorClient(500)
	_, err := client.Embed(context.Background(), []string{"text"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
