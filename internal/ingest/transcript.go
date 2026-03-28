package ingest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ErrTranscriptNotFound はトランスクリプトファイルが存在しない場合のエラー。
var ErrTranscriptNotFound = errors.New("transcript not found")

// Turn は正規化されたトランスクリプトのターンを表す。
type Turn struct {
	Role      string    // "user" | "assistant" | "tool"
	Content   string    // プレーンテキスト（tool call は簡略化）
	CreatedAt time.Time
}

// rawTurn は transcript JSONL の1行を表す。
type rawTurn struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	Timestamp string          `json:"timestamp"`
	UUID      string          `json:"uuid"`
}

// rawMessage は message フィールドの構造体。
type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentPart は content が配列の場合の各要素。
type contentPart struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// ParseTranscript はトランスクリプトファイルを []Turn に変換する。
// ファイルが存在しない場合は ErrTranscriptNotFound を返す。
// JSONL パースエラーの行はスキップして best-effort で処理する。
func ParseTranscript(path string) ([]Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrTranscriptNotFound
		}
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	var turns []Turn
	skipped := 0

	scanner := bufio.NewScanner(f)
	// 大きな行に対応するためバッファを拡張
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw rawTurn
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			skipped++
			continue
		}

		// type が user/assistant 以外はスキップ
		if raw.Type != "user" && raw.Type != "assistant" {
			continue
		}

		var msg rawMessage
		if err := json.Unmarshal(raw.Message, &msg); err != nil {
			skipped++
			continue
		}

		content, err := normalizeContent(msg.Content)
		if err != nil {
			skipped++
			continue
		}

		// 空 content はスキップ
		if strings.TrimSpace(content) == "" {
			continue
		}

		createdAt := parseTimestamp(raw.Timestamp)

		role := msg.Role
		if role == "" {
			role = raw.Type
		}

		turns = append(turns, Turn{
			Role:      role,
			Content:   content,
			CreatedAt: createdAt,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript: %w", err)
	}

	if skipped > 0 {
		// best-effort: スキップ行数をログには出さない（呼び出し元が必要なら別途）
		_ = skipped
	}

	return turns, nil
}

// normalizeContent は content フィールドを正規化してプレーンテキストに変換する。
func normalizeContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	// まず string として試みる
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}

	// 次に []contentPart として試みる
	var parts []contentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("unmarshal content: %w", err)
	}

	var sb strings.Builder
	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(part.Text)
			}
		case "tool_use":
			// tool_use は "[Tool: name(input_summary)]" に圧縮
			inputSummary := summarizeInput(part.Input)
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			fmt.Fprintf(&sb, "[Tool: %s(%s)]", part.Name, inputSummary)
		case "tool_result":
			// tool_result はスキップ
		}
	}

	return sb.String(), nil
}

// summarizeInput は tool_use の input を短い文字列に要約する。
func summarizeInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "..."
	}

	var parts []string
	for k, v := range m {
		vStr := fmt.Sprintf("%v", v)
		if len(vStr) > 50 {
			vStr = vStr[:50] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, vStr))
		if len(parts) >= 3 {
			break
		}
	}
	return strings.Join(parts, ", ")
}

// parseTimestamp は RFC3339 または RFC3339Nano 形式のタイムスタンプをパースする。
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	// JavaScript ISO 形式 (例: "2026-03-28T12:34:56.789Z") を処理
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05.000Z07:00",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}
