package service

import (
	"context"
	"errors"
	"fmt"
	"io"
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
func AgentChat(ctx context.Context, message *schema.Message, r *milvusRetriever.Retriever, onChunk func(string) error) error {
	var maxRounds = 3
	content := strings.TrimSpace(message.Content)
	if content == "" {
		log.Printf("请输入查询内容")
		return MessageEmpty
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

	resp_tool := NewSearchPostTool(r)
	tool_info, err := resp_tool.Info(ctx)
	if err != nil {
		log.Printf("获取tool_info失败:%v", err)
		return err
	}
	for round := 0; round < maxRounds; round++ { // 最多循环4轮
		stream_resp, err := llm.Chat(ctx, msg, []*schema.ToolInfo{tool_info})
		if err != nil {
			log.Printf("model 调用失败:%v", err)
			return err
		}
		MSG := []*schema.Message{}   // 接收流式输出的信息,包含toolcalls等
		var resp_msg *schema.Message //
		// 接收流式消息
		for {
			chunk, err := stream_resp.Recv()
			if errors.Is(err, io.EOF) {
				stream_resp.Close() // 关闭流
				log.Printf("信息接收完了")
				resp_msg, err = schema.ConcatMessages(MSG) // 和并流式输出结果成一个完整Messages
				if err != nil {
					log.Printf("合并失败:%v", err)
					return err
				}
				break
			}
			if err != nil {
				stream_resp.Close()
				return err
			}
			fmt.Printf("content:%v,长度为:%v", chunk.Content, len(chunk.ToolCalls))
			MSG = append(MSG, chunk)
			if chunk.Content != "" {
				err = onChunk(chunk.Content) // 将流式输出的内容(chunk)传给客户端
				if err != nil {
					stream_resp.Close()
					return err
				}
				continue
			}

		}

		// TODO:判断toocalls调用是否已经输出

		// 将流式信息合成一段完整信息
		// 它会读完并关闭流，同时合并文字和工具调用片段
		// resp_msg, err := schema.ConcatMessageStream(stream_resp)
		// if err != nil {
		// 	return nil, err
		// }

		// 模型没有选择调用tool 直接退出函数
		if len(resp_msg.ToolCalls) == 0 {
			return nil
		}
		msg = append(msg, resp_msg) // 调用tool时  继续追加msg调用msg
		for _, tool := range resp_msg.ToolCalls {
			// 封装为Eion-Tool
			if tool.Function.Name == "search_posts" {
				result, err := resp_tool.InvokableRun(ctx, tool.Function.Arguments) // 拿到包含post的string
				if err != nil {
					log.Printf("invokable failed:%v", err)
					return err
				}
				// 打印调用工具
				log.Printf("调用工具名:%v,参数:%v,result:%v", tool.Function.Name, tool.Function.Arguments, result)
				msg = append(msg, &schema.Message{
					Role:       schema.Tool,
					Content:    result,
					ToolCallID: tool.ID,
				})
			} else {
				return fmt.Errorf("暂不支持的工具:%s", tool.Function.Name)
			}
		}
	}
	return fmt.Errorf("达到最大模型调用轮次 %d，仍未获得最终回答", maxRounds)

}

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
