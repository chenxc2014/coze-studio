package agentflow

import (
	"context"
	"errors"
	"io"

	"github.com/cloudwego/eino/schema"
)

// 模型可能先返回“正在查询”等文字，再返回工具调用，不能将首段文字误判为最终答案。
// 有工具的智能体在未发现调用时需等到本轮流结束，普通无工具对话不经过此检查。
func streamContainsToolCalls(ctx context.Context, stream *schema.StreamReader[*schema.Message]) (bool, error) {
	defer stream.Close()
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		message, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if message != nil && len(message.ToolCalls) > 0 {
			return true, nil
		}
	}
}
