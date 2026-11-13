package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/llm"
	Model "micro_user/gateway/model"
	"strconv"
	"strings"
	"sync"
	"time"

	milvusRetriever "github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

type SearchPostsRequest struct {
	Keyword string `json:"keyword" jsonschema:"required"`
}

type GetPostRequest struct {
	Postid int64 `json:"post_id"`
}
type GetPostResult struct {
	Found bool
	Post  *Model.Post
}

type PostSource struct {
	Postid any `json:"post_id"`
	Title  any `json:"title"`
}

// 封装react.agent函数
func NewReActAgent(ctx context.Context, config *react.AgentConfig) (_ *react.Agent, err error) {
	agent, err := react.NewAgent(ctx, config)
	if err != nil {
		return nil, err
	}
	return agent, nil
}

// 接收用户完整信息,交给model拿到message(包含user,tool,assistant)
func AgentChat(ctx context.Context, message *schema.Message, r *milvusRetriever.Retriever, onChunk func(string) error) ([]PostSource, error) {
	var Mutex sync.Mutex
	model, err := llm.NewChatModel(ctx)
	if err != nil {
		return nil, err
	}

	// 回调函数		ondocs和onpost分别拿到数据库的数据和milvus中检索出的数据
	sources := []PostSource{}         // 存储来源信息
	seen := make(map[string]struct{}) // 用于去重

	onDocs := func(docs []*schema.Document) {
		Mutex.Lock()
		defer Mutex.Unlock()
		for _, doc := range docs { // 拿到milvus中查出的语义相近的doc  并去重后加入source
			_, exists := seen[doc.ID]
			if exists { // true 表示已存在
				continue // 退出这次循环,进行一下次
			}
			// 不存在(第一次需要追加到source)
			seen[doc.ID] = struct{}{}
			sources = append(sources, PostSource{Postid: doc.MetaData["post_id"], Title: doc.MetaData["title"]})
			log.Printf("检索来源 post_id=%v title=%v",
				doc.MetaData["post_id"], doc.MetaData["title"])
		}
	}

	onPost := func(post *Model.Post) {
		Mutex.Lock()
		defer Mutex.Unlock()
		key := "post_" + strconv.FormatInt(post.Id, 10)
		log.Printf("检索到:%v", key)
		_, exists := seen[key] // 检测key是否在seen中
		if exists {
			return
		}
		seen[key] = struct{}{}
		sources = append(sources, PostSource{Postid: post.Id, Title: post.Title})

	}

	ToolInfo1 := NewSearchPostTool(r, onDocs)
	ToolInfo2 := NewGetPostTool(r, onPost)

	cfg := &react.AgentConfig{
		ToolCallingModel: model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{ToolInfo1, ToolInfo2},
		},
		StreamToolCallChecker: func(ctx context.Context, modelOutput *schema.StreamReader[*schema.Message]) (bool, error) {
			defer modelOutput.Close()
			// TODO  这里如果用户问的是  "你好" 等不需要调用tool func 的问题 则必须等待model的流式信息全部收集完才会指向END
			for {
				chunk, err := modelOutput.Recv()
				if errors.Is(err, io.EOF) {
					log.Printf("checker EOF: %s", time.Now().Format("15:04:05.000"))
					return false, nil
				}
				if err != nil {
					return false, err
				}
				log.Printf("checker收到分片:时间:%v,内容长度:%v,工具调用数:%v,内容:%v", time.Now(), len(chunk.Content), len(chunk.ToolCalls), chunk.Content)

				if len(chunk.ToolCalls) != 0 {
					return true, nil // 继续指向工具节点,执行tool func
				}
				if chunk.Content != "" {
					if err := onChunk(chunk.Content); err != nil {
						return false, err
					}
				}
				// 当模型先输出正文后输出toolcalls时,换成下述
				// if chunk.Content != "" { // 遇到正文就返回(firstLineOfDefence) 这里需要确保是先返回toolcalls 后返回正文
				// 	return false, nil // 引流直接返回给用户
				// }

			}

		},
	}
	// 拿到post出处(post_id)
	agent, err := NewReActAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}

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
	// agent_msg, err := agent.Generate(ctx, msg)
	stream_msg, err := agent.Stream(ctx, msg) // 此时模型生成keyword和postid
	// stream_msg为模型最终返回的信息 Role=Assistant
	if err != nil {
		return nil, err
	}
	defer stream_msg.Close()

	for {
		_, err := stream_msg.Recv()
		if errors.Is(err, io.EOF) {
			fmt.Printf("接收完了信息")
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return sources, nil
}

// 根据用户帖子找到相近语义的数据
func NewSearchPostTool(r *milvusRetriever.Retriever, onDocs func([]*schema.Document)) tool.InvokableTool {
	handler := func(ctx context.Context, req *SearchPostsRequest) ([]*schema.Document, error) {
		log.Printf("执行 search_posts，keyword=%q", req.Keyword)
		docs, err := r.Retrieve(ctx, req.Keyword)
		if err != nil {
			return nil, err
		}
		onDocs(docs)
		return docs, err
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

// get_post_detail
// 根据用户询问的postid查看对应post内容
func NewGetPostTool(r *milvusRetriever.Retriever, onPost func(*Model.Post)) tool.InvokableTool {
	handler := func(ctx context.Context, req *GetPostRequest) (*GetPostResult, error) {
		log.Printf("执行 get_post，post_id=%d", req.Postid)
		post, err := dao.GetPostByID(ctx, req.Postid)
		if errors.Is(err, dao.ErrPostNotExist) {
			return &GetPostResult{Found: false}, nil
		}
		if err != nil {
			return nil, err
		}
		onPost(post)
		return &GetPostResult{Post: post, Found: true}, nil
	}
	tool := utils.NewTool(&schema.ToolInfo{
		Name: "get_post",
		Desc: "根据传入的id查找对应帖子的内容",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"post_id": &schema.ParameterInfo{
				Type:     schema.Integer,
				Required: true,
			},
		}),
	}, handler)
	return tool
}
