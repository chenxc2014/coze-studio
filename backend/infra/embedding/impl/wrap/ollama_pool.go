package wrap

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"github.com/cloudwego/eino/components/embedding"
	"golang.org/x/sync/errgroup"

	contract "github.com/coze-dev/coze-studio/backend/infra/embedding"
)

type ollamaPoolWorker struct {
	embedder embedding.Embedder
	busy     bool
}

type ollamaPool struct {
	dims      int64
	batchSize int
	workers   []*ollamaPoolWorker
	next      atomic.Uint64
	mu        sync.Mutex
	changed   chan struct{}
}

// NewOllamaPoolEmbedder 仅并行编码，完整且有序地返回结果后再交给现有索引流程写入。
func NewOllamaPoolEmbedder(ctx context.Context, configs []*ollama.EmbeddingConfig, dimensions int64, batchSize int) (contract.Embedder, error) {
	if len(configs) < 2 || len(configs) > 4 || dimensions <= 0 || batchSize <= 0 {
		return nil, fmt.Errorf("invalid ollama pool size, dimensions or batch size")
	}
	pool := &ollamaPool{dims: dimensions, batchSize: batchSize, changed: make(chan struct{})}
	for i, config := range configs {
		if config == nil || config.BaseURL == "" || config.Model == "" {
			return nil, fmt.Errorf("invalid ollama pool worker %d", i)
		}
		if i > 0 && config.Model != configs[0].Model {
			return nil, fmt.Errorf("ollama pool workers must use the same model")
		}
		cfg := *config
		if cfg.HTTPClient == nil && cfg.Timeout <= 0 {
			cfg.Timeout = 10 * time.Minute
		}
		emb, err := ollama.NewEmbedder(ctx, &cfg)
		if err != nil {
			return nil, fmt.Errorf("init ollama pool worker %d: %w", i, err)
		}
		pool.workers = append(pool.workers, &ollamaPoolWorker{embedder: emb})
	}
	return pool, nil
}

func (p *ollamaPool) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([][]float64, len(texts))
	if len(texts) == 0 {
		return result, nil
	}
	count := (len(texts) + p.batchSize - 1) / p.batchSize
	parallel := min(count, len(p.workers))
	var next atomic.Int64
	group, batchCtx := errgroup.WithContext(ctx)
	for i := 0; i < parallel; i++ {
		group.Go(func() error {
			for {
				index := int(next.Add(1)) - 1
				if index >= count {
					return nil
				}
				start := index * p.batchSize
				end := min(start+p.batchSize, len(texts))
				vectors, err := p.embedBatch(batchCtx, texts[start:end], opts...)
				if err != nil {
					return err
				}
				// 每个批次只写自己对应的位置，完成顺序不影响原始记录与向量的配对。
				copy(result[start:end], vectors)
			}
		})
	}
	if err := group.Wait(); err != nil {
		// 任一批次失败都不向上层暴露部分结果，现有消息重试仍使用原批次断点。
		return nil, err
	}
	return result, nil
}

func (p *ollamaPool) embedBatch(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	excluded := -1
	var lastErr error
	// 编码没有外部写入副作用；服务调用失败时，只在另一个进程补试一次。
	for attempt := 0; attempt < 2; attempt++ {
		index, worker, err := p.acquire(ctx, excluded)
		if err != nil {
			return nil, err
		}
		vectors, err := worker.embedder.EmbedStrings(ctx, texts, opts...)
		p.release(worker)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			excluded = index
			continue
		}
		if len(vectors) != len(texts) {
			return nil, fmt.Errorf("ollama pool returned %d vectors for %d texts", len(vectors), len(texts))
		}
		for _, vector := range vectors {
			if int64(len(vector)) != p.dims {
				return nil, fmt.Errorf("ollama pool vector dimensions do not match %d", p.dims)
			}
			for _, value := range vector {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return nil, fmt.Errorf("ollama pool returned non-finite vector")
				}
			}
		}
		return vectors, nil
	}
	return nil, fmt.Errorf("ollama pool batch failed on two workers: %w", lastErr)
}

// 优先使用空闲进程，不让较短批次排在另一个进程的慢批次后面。
func (p *ollamaPool) acquire(ctx context.Context, excluded int) (int, *ollamaPoolWorker, error) {
	start := int((p.next.Add(1) - 1) % uint64(len(p.workers)))
	for {
		p.mu.Lock()
		for offset := range p.workers {
			index := (start + offset) % len(p.workers)
			worker := p.workers[index]
			if index != excluded && !worker.busy {
				worker.busy = true
				p.mu.Unlock()
				return index, worker, nil
			}
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return -1, nil, ctx.Err()
		case <-changed:
		}
	}
}

func (p *ollamaPool) release(worker *ollamaPoolWorker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	worker.busy = false
	close(p.changed)
	p.changed = make(chan struct{})
}

func (p *ollamaPool) EmbedStringsHybrid(context.Context, []string, ...embedding.Option) ([][]float64, []map[int]float64, error) {
	return nil, nil, fmt.Errorf("ollama pool does not support hybrid embeddings")
}

func (p *ollamaPool) Dimensions() int64 { return p.dims }

func (p *ollamaPool) SupportStatus() contract.SupportStatus { return contract.SupportDense }
