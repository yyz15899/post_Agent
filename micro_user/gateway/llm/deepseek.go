package llm

import (
	"context"
	"log"
	"os"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/schema"
)

func NewChatModel(ctx context.Context) (*deepseek.ChatModel, error) {
	model, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey: os.Getenv("DEEPSEEK_API_KEY"),
		Model:  os.Getenv("MODEL_LLM"),
	})
	if err != nil {
		log.Printf("模型生成失败:%v", err)
		return nil, err
	}
	return model, nil
}

func Chat(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.StreamReader[*schema.Message], error) {
	// 这里message为用户的问题,比如“帮我找讲 gRPC 的帖子”
	chatmodel, err := NewChatModel(ctx) // 这里是ds的chat模型
	if err != nil {
		return nil, err
	}
	// 封装ToolInfo
	tool := tools
	toolmodel, err := chatmodel.WithTools(tool)
	if err != nil {
		log.Printf("工具绑定失败:%v", err)
		return nil, err
	}

	// 使用可以调用工具的model,返回模型输出
	// 非流式
	// resp, err := toolmodel.Generate(ctx, messages)
	// 流式
	resp, err := toolmodel.Stream(ctx, messages)
	if err != nil {
		log.Printf("模型调用失败,%v", err)
		return nil, err
	}

	return resp, nil
	// return resp, nil
}

// 一个不接收toolinfo的model调用(message中仍含有toolinfo,因为没有了withtool())
func ChatResp(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	chatmodel, err := NewChatModel(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := chatmodel.Stream(ctx, messages)
	if err != nil {
		log.Printf("模型调用失败,%v", err)
		return nil, err
	}
	return resp, nil
}
