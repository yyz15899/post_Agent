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
	answer, err := service.AgentChat(ctx, req, p.retriever)
	if err != nil {
		switch {
		case errors.Is(err, service.MessageEmpty):
			log.Printf("输入信息为空")
			c.JSON(http.StatusBadRequest, gin.H{"msg": "问题不能为空"})
		case errors.Is(err, context.DeadlineExceeded): // 会检测%w封装的内层错误
			c.JSON(http.StatusGatewayTimeout, gin.H{"msg": "模型响应超时，请稍后重试"})
		default:
			log.Printf("AgentChat failed,err:%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"msg": "服务暂时不可用"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"resp": answer.Content,
	})
}
