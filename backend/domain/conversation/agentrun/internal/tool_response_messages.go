package internal

import (
	"context"

	"github.com/cloudwego/eino/schema"
	message "github.com/coze-dev/coze-studio/backend/crossdomain/message/model"
	"github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
	msgEntity "github.com/coze-dev/coze-studio/backend/domain/conversation/message/entity"
)

// 一次并行工具事件可能携带多条结果，逐条构造存储消息，保留各自的 tool_call_id。
func buildToolResponseMessages(ctx context.Context, chunk *entity.AgentRespEvent, runtime *AgentRuntime, previous *msgEntity.Message, prefix string) []*message.Message {
	if chunk == nil {
		return nil
	}
	result := make([]*message.Message, 0, len(chunk.ToolsMessage))
	for _, response := range chunk.ToolsMessage {
		if response == nil {
			continue
		}
		one := *chunk
		one.ToolsMessage = []*schema.Message{response}
		msg := buildAgentMessage2Create(ctx, &one, message.MessageTypeToolResponse, runtime)
		// 只让第一条结果复用预创建的展示消息，其余结果使用独立消息 ID。
		if len(result) == 0 && previous != nil {
			msg.ID = previous.ID
			msg.CreatedAt = previous.CreatedAt
			msg.UpdatedAt = previous.UpdatedAt
			if prefix != "" {
				msg.Content = prefix + "\n" + msg.Content
			}
		}
		result = append(result, msg)
	}
	return result
}
