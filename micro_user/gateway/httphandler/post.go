package httphandler

import (
	"context"
	"errors"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/model"
	"micro_user/gateway/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
)

type PostHandler struct {
	voteStages *VoteStageMetrics // 耗时计算
}

func NewPostHandler(voteStages *VoteStageMetrics) *PostHandler {
	return &PostHandler{voteStages: voteStages}
}

func (p *PostHandler) CreatePost(c *gin.Context) {
	// 拿到body(json)
	i := new(model.CreatePostRequest)
	if err := c.ShouldBindJSON(i); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "输入内容不合法"})
		return
	}
	// 获取userid
	userid := c.GetInt64("user_id")
	if userid <= 0 {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "认证上下文缺失"})
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
		c.JSON(http.StatusBadRequest, gin.H{"msg": "帖子id必须为正数"})
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

func (p *PostHandler) VotePost(c *gin.Context) { // 只需要userid 不需要getprofile中的username(以此为优化点)
	// 取出公用的2s上下文
	ctx := c.Request.Context()

	// 追踪链路
	tracer := otel.Tracer("gateway_vote")

	post_id := c.Param("id") // 收到路径参数(postid)
	i := new(model.Direction)
	err := c.ShouldBindJSON(i) // 收到json内容(点赞还是踩)
	if err != nil || (i.Direction != 1 && i.Direction != -1) {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "Direction 只能是 1 或 -1"})
		return
	}
	pid, err := strconv.ParseInt(post_id, 10, 64)
	if err != nil || pid <= 0 {
		log.Printf("转换格式失败")
		c.JSON(http.StatusBadRequest, gin.H{"msg": "帖子id必须为正整数"})
		return
	}
	// stageStarted := time.Now()	耗时计算
	int_uid := c.GetInt64("user_id")
	if int_uid <= 0 {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"msg": "认证上下文缺失"})
		return
	}
	// 检查post是否存在
	// 追踪
	lookupCtx, lookupSpan := tracer.Start(ctx, "vote.post_lookup") // dao层
	_, err = dao.GetPostByID(lookupCtx, pid)                       // 耗时点02(查存)	这里将父span传入,用于后续计时
	lookupSpan.End()                                               // end到函数头部之间耗时被记录
	if errors.Is(err, context.DeadlineExceeded) {
		log.Printf("获取post信息超时,上下文关闭,%v", err)
		c.JSON(http.StatusGatewayTimeout, gin.H{"msg": "上下文超时关闭"})
		return
	}
	if errors.Is(err, dao.ErrPostNotExist) {
		log.Printf("post不存在:%v", err)
		c.JSON(http.StatusNotFound, gin.H{"msg": "请输入正确的postid"}) // 返回404
		return
	}
	if err != nil {
		log.Printf("查询post失败:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"msg": "服务繁忙"})
		return
	}

	// 开始计时
	lookupCtx, lookupSpan = tracer.Start(ctx, "vote.post_vote")

	err = service.VotePostByID(lookupCtx, int_uid, pid, i.Direction) // 耗时点03(写入票数,包括其中的dao层操作)
	lookupSpan.End()                                                 // 耗时
	if errors.Is(err, context.DeadlineExceeded) {
		log.Printf("投票处理超时,上下文关闭,:%v", err)
		c.JSON(http.StatusGatewayTimeout, gin.H{"msg": "上下文超时关闭"})
		return
	}
	if err != nil {
		log.Printf("投票失败:%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"msg": "投票失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"msg": "投票成功"})

}

// 总耗时
func (p *PostHandler) VoteTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只统计点赞接口
		if c.Request.Method != http.MethodPost ||
			c.FullPath() != "/api/posts/:id/vote" {
			c.Next()
			return
		}

		started := time.Now()

		defer func() {
			p.voteStages.Observe("handler_total", started)

			cost := time.Since(started)
			if cost >= 100*time.Millisecond {
				log.Printf(
					"slow_vote status=%d post_id=%s total_ms=%.2f",
					c.Writer.Status(),
					c.Param("id"),
					float64(cost)/float64(time.Millisecond),
				)
			}
		}()

		c.Next() // 执行后续的鉴权和handle(统计总时长)
	}
}

// 获取当前用户的点赞情况
func (p *PostHandler) GetScoreByID(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	pid := c.Param("pid")
	postid, err := strconv.ParseInt(pid, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "参数格式错误"})
		return
	}
	if postid <= 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "检查postid,必须大于0"})
		return
	}
	//查存
	_, err = dao.GetPostByID(ctx, postid)
	switch {
	case errors.Is(err, dao.ErrPostNotExist):
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"msg": "帖子不存在"})
		return
	case errors.Is(err, context.DeadlineExceeded):
		c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{"msg": "查询超时"})
		return
	case err != nil:
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "查询失败"})
		return
	}
	//
	score, err := service.GetScoreByID(ctx, postid)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "获取帖子总点赞失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"vote_num": score,
	})
}

// 查询当前用户当前查询的post的投票情况
func (p *PostHandler) GetMyVote(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	pid := c.Param("pid")
	postid, err := strconv.ParseInt(pid, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "参数格式错误"})
		return
	}
	if postid <= 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "检查postid,必须大于0"})
		return
	}
	// 查存
	_, err = dao.GetPostByID(ctx, postid)
	switch {
	case errors.Is(err, dao.ErrPostNotExist):
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"msg": "帖子不存在"})
		return
	case errors.Is(err, context.DeadlineExceeded):
		c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{"msg": "查询超时"})
		return
	case err != nil:
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "查询失败"})
		return
	}

	// 获取当前用户uid
	uid, ok := c.Get("user_id")
	if !ok { // 500(服务异常)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "没有查到相关信息"})
		return
	}
	// 查询vote
	score, err := service.GetScoreNow(ctx, uid.(int64), postid)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "获取帖子点赞数失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"direction": score,
	})

}
