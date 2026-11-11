/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentflow

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
	"github.com/coze-dev/coze-studio/backend/bizpkg/llm/modelbuilder"
	crossknowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge"
	knowledgeModel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	knowledgeEntity "github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/service"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/conv"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/slices"
)

type retrieverConfig struct {
	knowledgeConfig *bot_common.Knowledge
	chatModel       modelbuilder.BaseChatModel
}

func newKnowledgeRetriever(_ context.Context, conf *retrieverConfig) (*knowledgeRetriever, error) {
	return &knowledgeRetriever{
		knowledgeConfig: conf.knowledgeConfig,
		chatModel:       conf.chatModel,
	}, nil
}

type knowledgeRetriever struct {
	knowledgeConfig *bot_common.Knowledge
	chatModel       modelbuilder.BaseChatModel
}

func (r *knowledgeRetriever) Retrieve(ctx context.Context, req *AgentRequest) ([]*schema.Document, error) {
	if r.knowledgeConfig == nil || len(r.knowledgeConfig.KnowledgeInfo) == 0 {
		return nil, nil
	}

	knowledgeIDs := make([]int64, 0, len(r.knowledgeConfig.KnowledgeInfo))
	for _, v := range r.knowledgeConfig.KnowledgeInfo {
		id, err := strconv.ParseInt(v.GetId(), 10, 64)
		if err != nil {
			return nil, err
		}
		knowledgeIDs = append(knowledgeIDs, id)
	}

	kr, err := genKnowledgeRequest(ctx, knowledgeIDs, r.knowledgeConfig,
		req.Input.Content, req.History)
	if err != nil {
		return nil, err
	}
	kr.AutoAddress = true
	kr.ResolveAddress = newAddressResolver(r.chatModel, req.Input.Content, req.History)

	resp, err := crossknowledge.DefaultSVC().Retrieve(ctx, kr)
	if err != nil {
		return nil, err
	}

	docs, err := convertDocument(ctx, resp.RetrieveSlices)
	if err != nil {
		return nil, err
	}

	return docs, nil
}

func (r *knowledgeRetriever) PackRetrieveResultInfo(ctx context.Context, docs []*schema.Document) (string, error) {
	packedRes := strings.Builder{}
	for _, doc := range docs {
		if doc != nil && doc.MetaData["structured_address"] == "true" {
			packedRes.WriteString("以下是系统已自动读取的绑定知识库地址匹配资料。用户一次提问后直接综合回答，不展示页码、不要求用户翻页来驱动检索。依据匹配记录总数说明数量，不能把摘要条数当作总数；资料不完整时明确仅为摘要，不得声称完整名单。记录数不保证机构去重。本轮计数检索不附明细，不代表前轮没有检索到明细；不能仅据此断言前轮名单无依据或撤回名单，回答当前数量即可。无匹配直接说明，不换成其它地区或编造机构。需要澄清时先确认地址。不要展示内部编号或SQL。\n")
			break
		}
	}
	for idx, doc := range docs {
		if doc == nil {
			continue
		}
		packedRes.WriteString(fmt.Sprintf("---\nrecall slice %d: %s\n", idx+1, doc.Content))
	}
	return packedRes.String(), nil
}

func genKnowledgeRequest(_ context.Context, ids []int64, conf *bot_common.Knowledge,
	query string, history []*schema.Message,
) (*service.RetrieveRequest, error) {
	rr := &service.RetrieveRequest{
		Query:        query,
		ChatHistory:  history,
		KnowledgeIDs: ids,
		Strategy: &knowledgeEntity.RetrievalStrategy{
			TopK:     conf.TopK,
			MinScore: conf.MinScore,

			SelectType: func() knowledgeModel.SelectType {
				if conf.Auto != nil && *conf.Auto {
					return knowledgeModel.SelectTypeAuto
				}
				return knowledgeModel.SelectTypeOnDemand
			}(),

			SearchType: func() knowledgeModel.SearchType {
				if conf.SearchStrategy == nil {
					return knowledgeModel.SearchTypeSemantic
				}
				switch *conf.SearchStrategy {
				case bot_common.SearchStrategy_SemanticSearch:
					return knowledgeModel.SearchTypeSemantic
				case bot_common.SearchStrategy_FullTextSearch:
					return knowledgeModel.SearchTypeFullText
				case bot_common.SearchStrategy_HybirdSearch:
					return knowledgeModel.SearchTypeHybrid
				default:
					return knowledgeModel.SearchTypeSemantic
				}
			}(),

			EnableQueryRewrite: conf.RecallStrategy != nil && conf.RecallStrategy.UseRewrite != nil && *conf.RecallStrategy.UseRewrite,
			EnableRerank:       conf.RecallStrategy != nil && conf.RecallStrategy.UseRerank != nil && *conf.RecallStrategy.UseRerank,
			EnableNL2SQL:       conf.RecallStrategy != nil && conf.RecallStrategy.UseNl2sql != nil && *conf.RecallStrategy.UseNl2sql,
		},
	}

	return rr, nil
}

func convertDocument(_ context.Context, docSlice []*knowledgeModel.RetrieveSlice) ([]*schema.Document, error) {
	return slices.Transform(docSlice, func(a *knowledgeModel.RetrieveSlice) *schema.Document {
		doc := &schema.Document{
			ID:      strconv.FormatInt(a.Slice.ID, 10),
			Content: a.Slice.GetSliceContent(),
			MetaData: map[string]any{
				"dataset_id":    conv.Int64ToStr(a.Slice.KnowledgeID),
				"dataset_name":  a.Slice.Name,
				"document_id":   conv.Int64ToStr(a.Slice.DocumentID),
				"document_name": a.Slice.DocumentName,
			},
		}
		doc.WithScore(a.Score)
		if a.Slice.Extra != nil && a.Slice.Extra["structured_address"] == "true" {
			doc.MetaData["structured_address"] = "true"
		}
		return doc
	}), nil
}
