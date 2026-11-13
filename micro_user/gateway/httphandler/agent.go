package httphandler

import (
	"context"
	"errors"
	"log"
	"micro_user/gateway/model"
	"micro_user/gateway/service"
	"micro_user/pb"
	"net/http"
	"strings"
	"time"

	milvusRetriever "github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AgentHandler struct {
	client    pb.UserServiceClient
	retriever *milvusRetriever.Retriever
}

func NewAgentHandler(client pb.UserServiceClient, retriever *milvusRetriever.Retriever) *AgentHandler {
	return &AgentHandler{client: client, retriever: retriever}
}

func (p *AgentHandler) AgentChat(c *gin.Context) {
	var i = new(model.AgentChatRequest)
	if err := c.ShouldBindJSON(i); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "输入内容不合法"})
		return
	}
	// 鉴权
	token := c.GetHeader("Authorization") // 截取token
	parts := strings.Fields(token)        // 以空格分区
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		c.JSON(http.StatusUnauthorized, gin.H{
			"msg": "缺少或无效的认证信息",
		})
		return
	}
	authctx, authcancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer authcancel()
	authctx = metadata.AppendToOutgoingContext(authctx, "token", parts[1])
	_, err := p.client.GetProfile(authctx, &emptypb.Empty{}) // 这里还是用的2s上下文
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			c.JSON(http.StatusBadRequest, gin.H{"msg": "参数有误"})
		case codes.Canceled:
			c.JSON(http.StatusBadGateway, gin.H{"msg": "请求取消"})
		case codes.DeadlineExceeded:
			c.JSON(http.StatusGatewayTimeout, gin.H{"msg": "请求超时"})
		case codes.Unauthenticated:
			c.JSON(http.StatusUnauthorized, gin.H{
				"msg": "token无效或已过期,请重新登录",
			})
		default:
			log.Printf("获取对应信息失败,err:%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"msg": "获取信息失败"})
		}
		return
	}
	//重新设置为30s上下文
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	req := &schema.Message{
		Role:    schema.User,
		Content: i.Message,
	}

	// 定义回调函数
	onChunk := func(content string) error {
		if err := ctx.Err(); err != nil { // 这里用 父ctx 错误时 只有可能是客户端断开连接
			return err
		}
		c.SSEvent("chunk", content)
		c.Writer.Flush()
		return nil
	}

	postsources, err := service.AgentChat(ctx, req, p.retriever, onChunk)
	if err != nil {
		log.Printf("AgentChat failed: %v", err)

		if c.Request.Context().Err() != nil {
			return // 请求已经取消，不再发送
		}
		if c.Writer.Written() { // 用于判断当前 HTTP 响应是否已经开始向客户端写入（或已经发送）,这里表示客户端开始响应
			// True表示继续执行
			c.SSEvent("error", "回答中断，请稍后重试") // 当内部发生异常中断时发送一条这样的错误提示
			c.Writer.Flush()                 // 强制将缓冲区的数据刷给客户端（确保前端能立刻收到这条错误消息，而不是卡在缓冲区）
			return
		}

		// 客户端还没有响应时
		switch {
		case errors.Is(err, service.MessageEmpty):
			log.Printf("输入信息为空")
			c.JSON(http.StatusBadRequest, gin.H{"msg": "问题不能为空"})
			return
		case errors.Is(err, context.DeadlineExceeded): // 会检测%w封装的内层错误
			c.JSON(http.StatusGatewayTimeout, gin.H{"msg": "模型响应超时，请稍后重试"})
			return
		default:
			log.Printf("AgentChat failed,err:%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"msg": "服务暂时不可用"})
			return
		}
	}
	c.SSEvent("source", postsources)
	c.Writer.Flush()

}

// func (p *AgentHandler) TestStream(c *gin.Context) {
// 	c.Header("Content-Type", "text/event-stream")
// 	c.Header("Cache-Control", "no-cache")

// 	parts := []string{"第一段:哈哈哈", "第二段:嘻嘻"}
// 	index := 0

// 	c.Stream(func(w io.Writer) bool {
// 		if index >= len(parts) {
// 			return false // 返回false就停止
// 		}
// 		if index > 0 {
// 			time.Sleep(time.Second)
// 		}
// 		c.SSEvent("chunk", parts[index])
// 		index++
// 		return true // 返回true就继续调用
// 	})
// }
