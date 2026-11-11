package agentflow

import (
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"testing"
)

type addressChatModel struct {
	content string
	calls   int
}

func (m *addressChatModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.calls++
	return schema.AssistantMessage(m.content, nil), nil
}
func (m *addressChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func TestAddressResolverUsesCurrentAgentModelAndHistory(t *testing.T) {
	m := &addressChatModel{content: `{"handled":true,"province":"","city":"杭州市","district":"余杭区","address":"","count_only":false,"clarification":""}`}
	resolve := newAddressResolver(m, "那余杭区呢", []*schema.Message{schema.UserMessage("杭州西湖区的医疗机构有哪些"), schema.AssistantMessage("共有45条匹配记录。", nil)})
	for i := 0; i < 2; i++ {
		result, err := resolve(context.Background())
		if err != nil || !result.Handled || result.District != "余杭区" || result.City != "杭州市" || result.Clarification != "" {
			t.Fatalf("对话地址识别失败：%+v %v", result, err)
		}
	}
	if m.calls != 1 {
		t.Fatalf("同次检索不应重复调用模型：%d", m.calls)
	}
}

func TestAddressResolverRejectsInventedLocations(t *testing.T) {
	m := &addressChatModel{content: `{"handled":true,"province":"浙江省","city":"杭州市","district":"西湖区","address":"","page":1,"page_size":10,"count_only":true,"clarification":""}`}
	result, err := newAddressResolver(m, "杭州西湖区医疗机构有多少", nil)(context.Background())
	if err != nil || result.Clarification == "" {
		t.Fatalf("未阻止推断用户没有给出的省份：%+v %v", result, err)
	}
	m = &addressChatModel{content: `{"handled":true,"city":"杭州市","table":"user"}`}
	if _, err = newAddressResolver(m, "查杭州医疗机构", nil)(context.Background()); err == nil {
		t.Fatal("模型不应注入额外字段")
	}
}

func TestAddressResultGuidanceForAutomaticAnswer(t *testing.T) {
	r := &knowledgeRetriever{}
	output, err := r.PackRetrieveResultInfo(context.Background(), []*schema.Document{{Content: `{"匹配记录总数":45,"资料是否完整":true}`, MetaData: map[string]any{"structured_address": "true"}}})
	if err != nil || len(output) == 0 {
		t.Fatal("结构化地址结果缺少回答说明")
	}
}
