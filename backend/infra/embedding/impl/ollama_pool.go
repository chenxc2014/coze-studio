package impl

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/embedding/ollama"
)

// 连接池只匹配明确指定的入口与模型，避免后台改模型时仍误用旧模型进程。
func ollamaPoolConfigs(baseURL, model string, dimensions int64) ([]*ollama.EmbeddingConfig, error) {
	raw := strings.TrimSpace(os.Getenv("OLLAMA_EMBEDDING_WORKER_URLS"))
	if raw == "" {
		return nil, nil
	}
	scopeURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OLLAMA_EMBEDDING_POOL_BASE_URL")), "/")
	scopeModel := strings.TrimSpace(os.Getenv("OLLAMA_EMBEDDING_POOL_MODEL"))
	if scopeURL == "" || scopeModel == "" {
		return nil, fmt.Errorf("ollama pool requires an explicit base URL and model scope")
	}
	if strings.TrimRight(baseURL, "/") != scopeURL || model != scopeModel || dimensions <= 0 {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) < 2 || len(parts) > 4 {
		return nil, fmt.Errorf("ollama pool requires two to four distinct worker URLs")
	}
	seen := make(map[string]bool)
	configs := make([]*ollama.EmbeddingConfig, 0, len(parts))
	for _, part := range parts {
		address := strings.TrimRight(strings.TrimSpace(part), "/")
		u, err := url.Parse(address)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("invalid ollama pool worker URL")
		}
		u.Host = strings.ToLower(u.Host)
		key := u.String()
		if seen[key] {
			return nil, fmt.Errorf("duplicate ollama pool worker URL")
		}
		seen[key] = true
		configs = append(configs, &ollama.EmbeddingConfig{BaseURL: address, Model: model})
	}
	return configs, nil
}
