package httphandler

import (
	"context"
	"log"
	"micro_user/pb"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type UserHandler struct {
	client pb.UserServiceClient
}

// 实现main中的cli
func NewUserHandler(client pb.UserServiceClient) *UserHandler {
	return &UserHandler{client: client}
}

func (h *UserHandler) Signup(c *gin.Context) {
	// 解析json中数据
	p := new(pb.SignupRequest)
	if err := c.ShouldBindJSON(p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "用户名或密码错误,同时确认JSON合法"})
		return
	}
	// 超时设置
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	// 执行服务
	SignupResp, err := h.client.Signup(ctx, p)
	if err != nil {
		switch status.Code(err) {
		case codes.AlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"msg": "用户名已存在"})
		case codes.InvalidArgument:
			c.JSON(http.StatusBadRequest, gin.H{"msg": "注册参数不合法"})
		default:
			log.Printf("调用注册服务失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"msg": "注册失败"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"msg":     "注册成功",
		"user_id": SignupResp.UserId,
	})

}

func (h *UserHandler) Signin(c *gin.Context) {
	in := new(pb.SigninRequest)
	if err := c.ShouldBindJSON(in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "账号或密码错误或token不合法"})
		return
	}
	// 超时
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	SigninResp, err := h.client.Signin(ctx, in)
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			c.JSON(http.StatusBadRequest, gin.H{"msg": "用户名或密码错误"})
		case codes.Internal:
			c.JSON(http.StatusInternalServerError, gin.H{"msg": "服务繁忙"})
		case codes.Unauthenticated:
			c.JSON(http.StatusUnauthorized, gin.H{"msg": "请登录，或重新登录"})
		default:
			log.Printf("调用登录服务失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"msg": "登录失败"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"msg":     "登录成功",
		"user_id": SigninResp.UserId,
		"token":   SigninResp.Token,
	})

}

func (h *UserHandler) GetInfo(c *gin.Context) {
	// 设置并验证请求头中的token
	Authorization := c.GetHeader("Authorization")
	parts := strings.Fields(Authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") { // 无大小写比较字符串
		c.JSON(http.StatusUnauthorized, gin.H{"msg": "缺少或无效的认证信息"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	// 转交token给grpc
	ctx = metadata.AppendToOutgoingContext(ctx, "token", parts[1])
	Info, err := h.client.GetProfile(ctx, &emptypb.Empty{})
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
	c.JSON(http.StatusOK, gin.H{
		"msg":  "成功查询用户信息",
		"Info": Info,
	})
}
