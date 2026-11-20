package middleware

import (
	"context"
	"errors"
	"micro_user/gateway/dao"
	"micro_user/pb"
	"micro_user/usersvr/model"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

// 解析claims数据  拿到uid
func verify(tokenstr string, key []byte) (int64, error) {
	data := &jwt.RegisteredClaims{} // 接收解构后数据
	// 结构获取token
	token, err := jwt.ParseWithClaims(tokenstr, data, func(t *jwt.Token) (any, error) {
		return key, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return 0, status.Error(codes.Unauthenticated, "token无效或已过期")
	}
	uid, err := strconv.ParseInt(data.Subject, 10, 64)
	if err != nil || uid <= 0 {
		return 0, status.Error(codes.Unauthenticated, "用户标识无效")
	}
	return uid, nil
}

// 投票鉴权
func VoteAuth(key []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second) // 创建2s上下文
		defer cancel()
		c.Request = c.Request.WithContext(ctx) // 后续vote接口拿到
		// 获取http中的bearer
		authentication := c.GetHeader("Authorization")
		parts := strings.Fields(authentication)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": "缺少认证信息"})
			return
		}
		tokenstr := parts[1]
		// 检验
		uid, err := verify(tokenstr, key)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": "检查token是否合法"})
			return
		}
		// 检查用户是否存在(相比于调用GetProFile,省去一次grpc往返)
		var User model.User
		err = dao.DB.WithContext(ctx).
			Model(&model.User{}).
			Select("id").
			Where("id = ?", uid).
			Take(&User).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": "未找到响应用户"})
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{"msg": "服务超时"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "服务繁忙"})
			return
		}
		c.Set("user_id", uid)
		c.Next()
	}
}

//

// 鉴权中间件
func AuthInterceptor(client pb.UserServiceClient) func(*gin.Context) {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		// 单独操作点赞接口(公用2s上下文时限)
		if c.Request.Method == http.MethodPost && c.FullPath() == "/api/posts/:id/vote" {
			c.Request = c.Request.WithContext(ctx) // 本次请求携带创建的2s上下文
		}
		authorization := c.GetHeader("Authorization") // 请求头中拿到
		parts := strings.Fields(authorization)        // 分割
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"msg": "缺少认证信息",
			})
			return
		}
		tokenstr := parts[1]                                           // 留下载荷(携带claims等信息)
		ctx = metadata.AppendToOutgoingContext(ctx, "token", tokenstr) // kv存入ctx
		user, err := client.GetProfile(ctx, &emptypb.Empty{})
		if err != nil {
			switch status.Code(err) {
			case codes.Unauthenticated:
				c.AbortWithStatusJSON(http.StatusUnauthorized,
					gin.H{"msg": "Token无效或已过期"})
				return
			case codes.InvalidArgument:
				c.AbortWithStatusJSON(http.StatusUnauthorized,
					gin.H{"msg": "认证信息异常或用户不存在"})
				return

			case codes.Unavailable:
				c.AbortWithStatusJSON(http.StatusServiceUnavailable,
					gin.H{"msg": "认证服务暂时不可用"})
				return
			case codes.DeadlineExceeded:
				c.AbortWithStatusJSON(http.StatusGatewayTimeout,
					gin.H{"msg": "身份验证超时"})
				return
			default:
				c.AbortWithStatusJSON(http.StatusInternalServerError,
					gin.H{"msg": "认证服务异常"})
				return
			}
		}
		userid, err := strconv.ParseInt(user.UserId, 10, 64)
		if err != nil || userid <= 0 {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"msg": "用户服务返回的身份信息异常",
			})
			return
		}
		c.Set("user_id", userid) // 上下文中存入 kv对
		c.Set("user_profile", user)
		c.Next()
	}
}
