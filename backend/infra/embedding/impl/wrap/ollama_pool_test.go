package wrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/ollama"
)

func poolTestInputs(n int) []string {
	values := make([]string, n)
	for i := range values {
		values[i] = strconv.Itoa(i)
	}
	return values
}

func poolTestReply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Input []string `json:"input"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	vectors := make([][]float64, len(body.Input))
	for i, text := range body.Input {
		n, _ := strconv.Atoi(text)
		vectors[i] = []float64{float64(n), float64(n + 1)}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors})
}

func newPoolForTest(t *testing.T, handlerA, handlerB http.HandlerFunc, batch int) *ollamaPool {
	t.Helper()
	a := httptest.NewServer(handlerA)
	b := httptest.NewServer(handlerB)
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	pool, err := NewOllamaPoolEmbedder(context.Background(), []*ollama.EmbeddingConfig{
		{BaseURL: a.URL, Model: "test", Timeout: time.Second},
		{BaseURL: b.URL, Model: "test", Timeout: time.Second},
	}, 2, batch)
	if err != nil {
		t.Fatal(err)
	}
	return pool.(*ollamaPool)
}

func assertPoolVectors(t *testing.T, vectors [][]float64, n int) {
	t.Helper()
	if len(vectors) != n {
		t.Fatalf("返回条数错误：%d", len(vectors))
	}
	for i, v := range vectors {
		if len(v) != 2 || v[0] != float64(i) || v[1] != float64(i+1) {
			t.Fatalf("第 %d 行向量顺序或内容错误：%v", i, v)
		}
	}
}

func TestOllamaPoolOrderAndGlobalConcurrency(t *testing.T) {
	var active, peak atomic.Int32
	var perWorker [2]atomic.Int32
	var violation atomic.Bool
	handler := func(index int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if perWorker[index].Add(1) > 1 {
				violation.Store(true)
			}
			defer perWorker[index].Add(-1)
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
			}
			if index == 0 {
				time.Sleep(12 * time.Millisecond)
			} else {
				time.Sleep(3 * time.Millisecond)
			}
			poolTestReply(w, r)
		}
	}
	pool := newPoolForTest(t, handler(0), handler(1), 3)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vectors, err := pool.EmbedStrings(context.Background(), poolTestInputs(11))
			if err != nil {
				t.Error(err)
				return
			}
			assertPoolVectors(t, vectors, 11)
		}()
	}
	wg.Wait()
	if violation.Load() || peak.Load() != 2 {
		t.Fatalf("全局并发或单进程并发错误：%d", peak.Load())
	}
	if pool.Dimensions() != 2 {
		t.Fatal("维度错误")
	}
}

func TestOllamaPoolFailoverAndRetry(t *testing.T) {
	var failedCalls atomic.Int32
	pool := newPoolForTest(t, func(w http.ResponseWriter, r *http.Request) {
		failedCalls.Add(1)
		http.Error(w, "temporary unavailable", 503)
	}, poolTestReply, 2)
	vectors, err := pool.EmbedStrings(context.Background(), poolTestInputs(9))
	if err != nil {
		t.Fatal(err)
	}
	assertPoolVectors(t, vectors, 9)
	if failedCalls.Load() == 0 {
		t.Fatal("未覆盖故障进程")
	}
	var broken atomic.Bool
	broken.Store(true)
	handler := func(w http.ResponseWriter, r *http.Request) {
		if broken.Load() {
			http.Error(w, "temporary unavailable", 503)
			return
		}
		poolTestReply(w, r)
	}
	pool = newPoolForTest(t, handler, handler, 2)
	vectors, err = pool.EmbedStrings(context.Background(), poolTestInputs(9))
	if err == nil || vectors != nil {
		t.Fatal("双进程故障时不得返回部分向量")
	}
	broken.Store(false)
	vectors, err = pool.EmbedStrings(context.Background(), poolTestInputs(9))
	if err != nil {
		t.Fatal(err)
	}
	assertPoolVectors(t, vectors, 9)
}

func TestOllamaPoolUsesAvailableWorker(t *testing.T) {
	var fastCalls atomic.Int32
	var once sync.Once
	release := make(chan struct{})
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		poolTestReply(w, r)
	}
	fast := func(w http.ResponseWriter, r *http.Request) {
		if fastCalls.Add(1) >= 5 {
			once.Do(func() { close(release) })
		}
		poolTestReply(w, r)
	}
	pool := newPoolForTest(t, slow, fast, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	vectors, err := pool.EmbedStrings(ctx, poolTestInputs(6))
	if err != nil {
		t.Fatalf("空闲进程未继续处理后续批次：%v", err)
	}
	assertPoolVectors(t, vectors, 6)
}

func TestOllamaPoolCancelWhileWaiting(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var released atomic.Bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !released.Load() {
			entered <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		poolTestReply(w, r)
	}
	pool := newPoolForTest(t, handler, handler, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := pool.EmbedStrings(ctx, poolTestInputs(2)); finished <- err }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("未启动并行请求")
		}
	}
	waitCtx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	vectors, err := pool.EmbedStrings(waitCtx, poolTestInputs(2))
	if !errors.Is(err, context.DeadlineExceeded) || vectors != nil {
		t.Fatalf("等待工作进程时取消失败：%v", err)
	}
	cancel()
	select {
	case err = <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("取消后未释放工作进程")
	}
	released.Store(true)
	close(release)
	vectors, err = pool.EmbedStrings(context.Background(), poolTestInputs(3))
	if err != nil {
		t.Fatal(err)
	}
	assertPoolVectors(t, vectors, 3)
}

func TestOllamaPoolRejectsInvalidVectors(t *testing.T) {
	for _, body := range []string{`{"embeddings":[]}`, `{"embeddings":[[1]]}`, `{"embeddings":[[1,2],[3,4]]}`} {
		t.Run(body, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, body)
			}
			pool := newPoolForTest(t, handler, handler, 1)
			vectors, err := pool.EmbedStrings(context.Background(), []string{"0"})
			if err == nil || vectors != nil {
				t.Fatal("无效向量不得交给索引流程")
			}
		})
	}
}

func TestOllamaPoolEmptyAndInvalidConfiguration(t *testing.T) {
	pool := newPoolForTest(t, poolTestReply, poolTestReply, 2)
	vectors, err := pool.EmbedStrings(context.Background(), nil)
	if err != nil || len(vectors) != 0 {
		t.Fatal("空输入处理错误")
	}
	configs := []*ollama.EmbeddingConfig{{BaseURL: "http://localhost:1", Model: "test"}, {BaseURL: "http://localhost:2", Model: "test"}}
	for _, cfg := range []struct {
		workers []*ollama.EmbeddingConfig
		dims    int64
		batch   int
	}{{configs, 2, 0}, {configs, 0, 2}, {configs[:1], 2, 2}, {[]*ollama.EmbeddingConfig{nil, nil}, 2, 2}} {
		if _, err := NewOllamaPoolEmbedder(context.Background(), cfg.workers, cfg.dims, cfg.batch); err == nil {
			t.Fatal("接受了错误配置")
		}
	}
}
