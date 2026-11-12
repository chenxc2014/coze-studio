package impl

import "testing"

func TestOllamaPoolConfigurationScope(t *testing.T) {
	t.Setenv("OLLAMA_EMBEDDING_WORKER_URLS", "http://127.0.0.1:11439,http://127.0.0.1:11440")
	t.Setenv("OLLAMA_EMBEDDING_POOL_BASE_URL", "http://172.18.0.1:11434/")
	t.Setenv("OLLAMA_EMBEDDING_POOL_MODEL", "bge-m3-cpu")
	configs, err := ollamaPoolConfigs("http://172.18.0.1:11434", "bge-m3-cpu", 1024)
	if err != nil || len(configs) != 2 || configs[1].Model != "bge-m3-cpu" {
		t.Fatalf("连接池配置错误：%v", err)
	}
	for _, test := range []struct {
		base, model string
		dims        int64
	}{{"http://other:11434", "bge-m3-cpu", 1024}, {"http://172.18.0.1:11434", "other-model", 1024}, {"http://172.18.0.1:11434", "bge-m3-cpu", 0}} {
		configs, err = ollamaPoolConfigs(test.base, test.model, test.dims)
		if err != nil || configs != nil {
			t.Fatal("不匹配的配置必须继续走原模型入口")
		}
	}
	t.Setenv("OLLAMA_EMBEDDING_WORKER_URLS", "")
	if configs, err = ollamaPoolConfigs("http://172.18.0.1:11434", "bge-m3-cpu", 1024); err != nil || configs != nil {
		t.Fatal("关闭连接池失败")
	}
}

func TestOllamaPoolRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("OLLAMA_EMBEDDING_POOL_BASE_URL", "http://base:11434")
	t.Setenv("OLLAMA_EMBEDDING_POOL_MODEL", "test")
	for _, value := range []string{"http://one:1", "http://one:1,http://one:1/", "http://one:1,file:///tmp/test", "http://one:1,http://u:p@two:2", "http://one:1,http://two:2?query=1", "http://one:1,"} {
		t.Setenv("OLLAMA_EMBEDDING_WORKER_URLS", value)
		if _, err := ollamaPoolConfigs("http://base:11434", "test", 1024); err == nil {
			t.Fatal("未拒绝错误连接池地址")
		}
	}
	t.Setenv("OLLAMA_EMBEDDING_POOL_BASE_URL", "")
	if _, err := ollamaPoolConfigs("http://base:11434", "test", 1024); err == nil {
		t.Fatal("必须指定模型作用范围")
	}
}
