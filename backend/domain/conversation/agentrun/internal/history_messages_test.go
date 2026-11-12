package internal

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	message "github.com/coze-dev/coze-studio/backend/crossdomain/message/model"
	"github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
	msgEntity "github.com/coze-dev/coze-studio/backend/domain/conversation/message/entity"
)

func historyCall(ids ...string) *schema.Message {
	msg := &schema.Message{Role: schema.Assistant}
	for _, id := range ids {
		msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{ID: id, Type: "function", Function: schema.FunctionCall{Name: "test_tool", Arguments: `{}`}})
	}
	return msg
}
func historyResponse(id string) *schema.Message {
	return &schema.Message{Role: schema.Tool, ToolCallID: id, Content: "合成工具结果"}
}

func TestToolHistoryPairsByCallID(t *testing.T) {
	cases := []struct {
		name  string
		input []*schema.Message
		roles []schema.RoleType
		calls []string
	}{
		{"并行两个调用和两个结果均保留", []*schema.Message{historyCall("a", "b"), historyResponse("b"), historyResponse("a")}, []schema.RoleType{schema.Assistant, schema.Tool, schema.Tool}, []string{"a", "b"}},
		{"缺失第二个结果时只保留已完成调用", []*schema.Message{historyCall("a", "b"), historyResponse("a")}, []schema.RoleType{schema.Assistant, schema.Tool}, []string{"a"}},
		{"消息数为偶数但ID不匹配时不能配对", []*schema.Message{historyCall("a"), historyResponse("b")}, nil, nil},
		{"孤立结果不进入模型历史", []*schema.Message{historyResponse("a"), schema.UserMessage("继续")}, []schema.RoleType{schema.User}, nil},
		{"不能跨用户消息匹配结果", []*schema.Message{historyCall("a"), schema.UserMessage("下一问"), historyResponse("a")}, []schema.RoleType{schema.User}, nil},
		{"重复结果只保留一条", []*schema.Message{historyCall("a"), historyResponse("a"), historyResponse("a")}, []schema.RoleType{schema.Assistant, schema.Tool}, []string{"a"}},
		{"无工具的正文和空记录兼容", []*schema.Message{nil, schema.UserMessage("问题"), schema.AssistantMessage("回答", nil)}, []schema.RoleType{schema.User, schema.Assistant}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := json.Marshal(tc.input)
			result := normalizeToolHistory(tc.input)
			var roles []schema.RoleType
			var calls []string
			for _, msg := range result {
				roles = append(roles, msg.Role)
				for _, call := range msg.ToolCalls {
					calls = append(calls, call.ID)
				}
			}
			if !reflect.DeepEqual(roles, tc.roles) || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("配对错误 roles=%v calls=%v", roles, calls)
			}
			after, _ := json.Marshal(tc.input)
			if string(before) != string(after) {
				t.Fatal("修复模型输入不应修改原历史")
			}
		})
	}
}

func TestToolHistoryDatabaseConversion(t *testing.T) {
	var stored []*message.Message
	for _, item := range []struct {
		msg  *schema.Message
		kind message.MessageType
	}{
		{historyCall("a", "b"), message.MessageTypeFunctionCall},
		{historyResponse("a"), message.MessageTypeToolResponse},
		{historyResponse("b"), message.MessageTypeToolResponse},
	} {
		data, _ := json.Marshal(item.msg)
		stored = append(stored, &message.Message{RunID: 10, MessageType: item.kind, ModelContent: string(data)})
	}
	stored = append(stored, nil, &message.Message{ModelContent: "null"}, &message.Message{ModelContent: "bad JSON"})
	result := transMessageToSchemaMessage(context.Background(), stored, nil)
	if len(result) != 3 || len(result[0].ToolCalls) != 2 {
		t.Fatal("完整的并行调用不应因为消息数为奇数而被删掉")
	}
	// 模拟线上遗留记录：工具调用声明两项，但存储中只有第一项结果。
	result = transMessageToSchemaMessage(context.Background(), stored[:2], nil)
	if len(result) != 2 || len(result[0].ToolCalls) != 1 || result[0].ToolCalls[0].ID != "a" {
		t.Fatal("遗留的不完整历史仍会让模型拒绝请求")
	}
}

func TestToolHistoryKeepsAssistantTextAndRoundBoundaries(t *testing.T) {
	msg := historyCall("missing")
	msg.Content = "原有说明"
	input := []*schema.Message{msg, schema.UserMessage("第二问"), historyCall("same"), historyResponse("same"), schema.UserMessage("第三问"), historyCall("same"), historyResponse("same")}
	result := normalizeToolHistory(input)
	if len(result) != 7 || result[0].Content != "原有说明" || len(result[0].ToolCalls) != 0 || len(result[2].ToolCalls) != 1 || len(result[5].ToolCalls) != 1 {
		t.Fatal("正文应保留，不同轮的相同工具ID应分别配对")
	}
}

func TestToolHistoryPersistsEveryParallelResponse(t *testing.T) {
	runtime := &AgentRuntime{RunRecord: &entity.RunRecordMeta{ID: 10}, RunMeta: &entity.AgentRunMeta{ConversationID: 11, AgentID: 12, SectionID: 13, UserID: "14"}, StartTime: time.Now()}
	chunk := &entity.AgentRespEvent{ToolsMessage: []*schema.Message{nil, historyResponse("a"), historyResponse("b")}}
	previous := &msgEntity.Message{ID: 99, CreatedAt: 100, UpdatedAt: 101}
	msgs := buildToolResponseMessages(context.Background(), chunk, runtime, previous, "流式展示前缀")
	if len(msgs) != 2 || msgs[0].ID != 99 || msgs[1].ID != 0 || msgs[0].CreatedAt != 100 || msgs[0].Content != "流式展示前缀\n合成工具结果" || msgs[1].Content != "合成工具结果" {
		t.Fatal("并行结果必须分别保存，只有第一条复用预创建消息")
	}
	callJSON, _ := json.Marshal(historyCall("a", "b"))
	stored := []*message.Message{{MessageType: message.MessageTypeFunctionCall, ModelContent: string(callJSON)}}
	stored = append(stored, msgs...)
	result := transMessageToSchemaMessage(context.Background(), stored, nil)
	if len(result) != 3 || result[1].ToolCallID != "a" || result[2].ToolCallID != "b" {
		t.Fatal("保存后的并行工具历史必须可供下一轮模型调用")
	}
	if len(chunk.ToolsMessage) != 3 || previous.ID != 99 {
		t.Fatal("不能修改原事件或预创建消息")
	}
}
