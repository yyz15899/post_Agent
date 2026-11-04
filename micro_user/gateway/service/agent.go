package service

import (
	"context"
	"fmt"
	"log"
	"micro_user/gateway/llm"
	"strings"

	milvusRetriever "github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type SearchPostsRequest struct {
	Keyword string `json:"keyword" jsonschema:"required"`
}

// 接收用户完整信息,交给model拿到message(包含user,tool,assistant)
func AgentChat(ctx context.Context, message *schema.Message, r *milvusRetriever.Retriever) (*schema.Message, error) {
	var maxRounds = 3
	content := strings.TrimSpace(message.Content)
	if content == "" {
		log.Printf("请输入查询内容")
		return nil, MessageEmpty
	}
	//将用户单条信息组织为切片
	msg := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是本站的一个智能Agent 用于帮助用户根据输入的信息找到相关帖子,如果工具返回空结果，明确告知本次未找到匹配帖子，不要编造；没有必要时不要反复搜索",
		}, {
			Role:    message.Role,
			Content: message.Content,
		},
	}

	resp_tool := NewSearchPostTool(r) // 创建Tool
	tool_info, err := resp_tool.Info(ctx)
	if err != nil {
		log.Printf("获取tool_info失败:%v", err)
		return nil, err
	}
	for round := 0; round < maxRounds; round++ { // 最多循环4轮
		resp_msg, err := llm.Chat(ctx, msg, []*schema.ToolInfo{tool_info})
		if err != nil {
			log.Printf("model 调用失败:%v", err)
			return nil, err
		}
		// 模型没有选择调用tool 直接返回结果
		if len(resp_msg.ToolCalls) == 0 {
			return resp_msg, nil
		}
		msg = append(msg, resp_msg) // 调用tool时  继续追加msg调用msg
		for _, tool := range resp_msg.ToolCalls {
			// 封装为Eion-Tool
			if tool.Function.Name == "search_posts" {
				result, err := resp_tool.InvokableRun(ctx, tool.Function.Arguments) // 拿到包含post的string
				if err != nil {
					log.Printf("invokable failed:%v", err)
					return nil, err
				}
				// 打印调用工具
				log.Printf("调用工具名:%v,参数:%v,result:%v", tool.Function.Name, tool.Function.Arguments, result)
				msg = append(msg, &schema.Message{
					Role:       schema.Tool,
					Content:    result,
					ToolCallID: tool.ID,
				})
			} else {
				return &schema.Message{}, fmt.Errorf("暂不支持的工具:%s", tool.Function.Name)
			}
		}
	}
	return nil, fmt.Errorf("达到最大模型调用轮次 %d，仍未获得最终回答", maxRounds)

}

// 封装调用post工具函数(后续直接走milvus检索数据,可以删除)
// func SearchPostsTool(ctx context.Context, req *SearchPostsRequest) ([]model.Post, error) {
// 	posts, err := SearchPosts(ctx, req.Keyword)
// 	if err != nil {
// 		log.Printf("调用工具失败:%v", err)
// 		return nil, err
// 	}

// 	return posts, nil
// }

// 封装searchpost工具的创建
func NewSearchPostTool(r *milvusRetriever.Retriever) tool.InvokableTool {
	handler := func(ctx context.Context, req *SearchPostsRequest) ([]*schema.Document, error) {
		return r.Retrieve(ctx, req.Keyword)
	} // 将与keyword语义相近的post从milvus中拿出(document形式)
	tool := utils.NewTool(&schema.ToolInfo{
		Name: "search_posts",
		Desc: "根据关键词搜索站内帖子",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"keyword": &schema.ParameterInfo{
				Type:     schema.String,
				Required: true,
			},
		}),
	}, handler) // 这里绑定调用invokable时的执行函数(参数形式需匹配)
	return tool
}
