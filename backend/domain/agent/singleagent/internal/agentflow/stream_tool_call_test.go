package agentflow

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestStreamToolCallsAfterText(t *testing.T) {
	stream := schema.StreamReaderFromArray([]*schema.Message{
		{Role: schema.Assistant, Content: "我将搜索这家机构的公开信息。"},
		{Role: schema.Assistant, Content: "正在核对官网"},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Function: schema.FunctionCall{Name: "search_medical_web", Arguments: `{"query":"诸暨市第五人民医院"}`}}}},
	})
	called, err := streamContainsToolCalls(context.Background(), stream)
	if err != nil || !called {
		t.Fatalf("文字之后的工具调用被忽略：called=%v err=%v", called, err)
	}
}

func TestStreamTextWithoutTools(t *testing.T) {
	stream := schema.StreamReaderFromArray([]*schema.Message{nil, {Role: schema.Assistant, Content: "这是完整答案"}})
	called, err := streamContainsToolCalls(context.Background(), stream)
	if err != nil || called {
		t.Fatalf("纯文字不能触发工具执行：called=%v err=%v", called, err)
	}
}

func TestStreamToolCallErrorAndCancel(t *testing.T) {
	want := errors.New("模型流中断")
	stream, writer := schema.Pipe[*schema.Message](1)
	writer.Send(nil, want)
	writer.Close()
	if called, err := streamContainsToolCalls(context.Background(), stream); called || !errors.Is(err, want) {
		t.Fatalf("应传播模型流错误：called=%v err=%v", called, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream = schema.StreamReaderFromArray([]*schema.Message{})
	if called, err := streamContainsToolCalls(ctx, stream); called || !errors.Is(err, context.Canceled) {
		t.Fatalf("应尊重取消：called=%v err=%v", called, err)
	}
}
