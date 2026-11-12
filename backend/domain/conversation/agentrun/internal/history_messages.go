package internal

import "github.com/cloudwego/eino/schema"

// normalizeToolHistory 按实际 tool_call_id 修复发送给模型的历史快照，不修改数据库原记录。
// 并行调用可能有多个结果，不能用同一轮消息的奇偶数判断配对，也不能伪造缺失的结果。
func normalizeToolHistory(messages []*schema.Message) []*schema.Message {
	result := make([]*schema.Message, 0, len(messages))
	for i := 0; i < len(messages); {
		msg := messages[i]
		if msg == nil || msg.Role == schema.Tool {
			i++
			continue
		}
		if msg.Role != schema.Assistant || len(msg.ToolCalls) == 0 {
			result = append(result, msg)
			i++
			continue
		}
		end := i + 1
		responses := make(map[string]*schema.Message)
		for end < len(messages) {
			response := messages[end]
			if response == nil {
				end++
				continue
			}
			if response.Role != schema.Tool {
				break
			}
			if response.ToolCallID != "" && responses[response.ToolCallID] == nil {
				responses[response.ToolCallID] = response
			}
			end++
		}
		paired := make(map[string]bool)
		calls := make([]schema.ToolCall, 0, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			if call.ID != "" && responses[call.ID] != nil && !paired[call.ID] {
				calls = append(calls, call)
				paired[call.ID] = true
			}
		}
		copyMsg := *msg
		copyMsg.ToolCalls = calls
		if len(calls) > 0 {
			result = append(result, &copyMsg)
			for j := i + 1; j < end; j++ {
				response := messages[j]
				if response != nil && paired[response.ToolCallID] {
					result = append(result, response)
					delete(paired, response.ToolCallID)
				}
			}
		} else if copyMsg.Content != "" || len(copyMsg.MultiContent) > 0 {
			// 工具未返回时仅保留模型原有的正文，不把未执行的调用当作已经完成。
			result = append(result, &copyMsg)
		}
		i = end
	}
	return result
}
