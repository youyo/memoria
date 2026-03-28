package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// Client は embedding worker への HTTP クライアント。
// 本番では UDS 経由、テストでは TCP サーバーへ接続する。
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// HealthResponse は /health エンドポイントのレスポンス。
type HealthResponse struct {
	Status     string `json:"status"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
	Device     string `json:"device"`
}

// EmbedRequest は /embed エンドポイントへのリクエスト。
type EmbedRequest struct {
	Texts []string `json:"texts"`
}

// EmbedResponse は /embed エンドポイントのレスポンス。
type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Model      string      `json:"model"`
	Dimensions int         `json:"dimensions"`
}

// New は UDS パスを指定して Client を生成する。
// 本番コードで使用する。
func New(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		baseURL:    "http://localhost",
		httpClient: &http.Client{Transport: transport},
	}
}

// NewWithHTTPClient はカスタム HTTP クライアントと baseURL を指定して Client を生成する。
// テストで TCP モックサーバーを注入するために使用する。
func NewWithHTTPClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// Health は /health エンドポイントを呼び出す。
// worker が応答しない場合は error を返す。
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("create health request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("health request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("decode health response: %w", err)
	}

	return &health, nil
}

// Embed は texts を embedding し、[][]float32 を返す。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// 空スライスの場合は即座に返す
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	body, err := json.Marshal(EmbedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed returned status %d", resp.StatusCode)
	}

	var embedResp EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}

	return embedResp.Embeddings, nil
}
