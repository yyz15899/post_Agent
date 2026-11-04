package httphandler

import (
	"context"
	"errors"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/model"
	"micro_user/gateway/service"
	"micro_user/pb"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PostHandler struct {
	client pb.UserServiceClient
}

func NewPostHandler(client pb.UserServiceClient) *PostHandler {
	return &PostHandler{client: client}
}

func (p *PostHandler) CreatePost(c *gin.Context) {
	// 拿到body
	i := new(model.CreatePostRequest)
	if err := c.ShouldBindJSON(i); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "输入内容不合法"})
		return
	}
	// 设置并拿到token
	token := c.GetHeader("Authorization")
	parts := strings.Fields(token)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") { // 无大小写比较字符串
		c.JSON(http.StatusUnauthorized, gin.H{"msg": "缺少或无效的认证信息"})
		return
	}
	// 拿到authorid  设置鉴权＋获取userinfo 超时  (后续的CreatPost操作涉及milvus数据库操作,2s可能不够)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "token", parts[1])

	profile, err := p.client.GetProfile(ctx, &emptypb.Empty{})
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
	// 查询authorid
	userid, err := strconv.ParseInt(profile.UserId, 10, 64)
	if err != nil {
		log.Printf("转换格式失败,err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"msg": "用户信息异常"})
		return
	}
	// 新建一个超时
	postCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	postid, err := service.CreatePost(postCtx, userid, i.Title, i.Content) // authorid不能用用户传入的
	if err != nil {
		log.Printf("帖子创建失败,err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"msg": "帖子创建未成功"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"msg":    "创建成功",
		"postid": postid,
	})
}

func (p *PostHandler) GetPostByID(c *gin.Context) {
	pid := c.Param("id")
	postid, err := strconv.ParseInt(pid, 10, 64)
	if err != nil || postid <= 0 {
		log.Printf("转换格式失败,err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"msg": "帖子id必须为正数"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	post, err := service.GetPostByID(ctx, postid)
	if errors.Is(err, dao.ErrPostNotExist) {
		log.Printf("帖子不存在")
		c.JSON(http.StatusNotFound, gin.H{"msg": "帖子未找到"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "帖子信息异常"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"msg":  "查找成功",
		"info": post,
	})
}

func (p *PostHandler) SearchPosts(c *gin.Context) {
	keyword := c.Query("keyword")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	posts, err := service.SearchPosts(ctx, keyword)
	if errors.Is(err, service.KeywordEmpty) {
		log.Printf("关键词为空")
		c.JSON(http.StatusBadRequest, gin.H{"msg": "关键词不能为空"})
		return
	}
	if err != nil {
		log.Printf("搜索帖子失败,err:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"msg": "服务繁忙"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"msg":   "查找到相关帖子",
		"posts": posts,
	})
}
