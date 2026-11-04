package middleware

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"micro_user/usersvr/config"
)

type uidKey struct{} // context 的 key 永远放包级别

// 校验token是否合法且正确
func verify(tokenstr string) (int64, error) {
	// 准备一个与载荷同类型对象用于接收claims数据(token)
	data := &jwt.RegisteredClaims{}
	// ParseWithClaims 作用是将tokenstr写入到claims结构体中
	token, err := jwt.ParseWithClaims(tokenstr, data, // 第二个入参作为写入数据的对象
		func(t *jwt.Token) (any, error) {
			return config.LocalSecretKey, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return 0, status.Error(codes.Unauthenticated, "token无效或已过期")
	}
	// 发送uid
	uid, err := strconv.ParseInt(data.Subject, 10, 64)
	if err != nil || uid <= 0 {
		return 0, status.Error(codes.Unauthenticated, "用户标识无效")
	}
	return uid, nil
}

func AuthInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	// 设置白名单
	// fmt.Printf("FullMethod:%v\n", info.FullMethod)		// 打印出 /pb.UserService/Signup
	if info.FullMethod == "/pb.UserService/Signup" || info.FullMethod == "/pb.UserService/Signin" {
		return handler(ctx, req)
	}
	md, ok := metadata.FromIncomingContext(ctx)
	// 这个 bool 的意思是：这条请求的上下文里到底有没有挂 metadata。
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "缺少认证信息")
	}
	Intoken := md.Get("token")
	if len(Intoken) == 0 {
		return nil, status.Error(codes.Unauthenticated, "token不能为空")
	}
	tokenStr := Intoken[0] // 先判空  再取下标,这里取出来的token由三部分以.隔开
	//	解码token
	userid, err := verify(tokenStr)
	if err != nil {
		log.Printf("token 校验未通过: %v", err)
		return nil, err
	}
	// 将解码后的token  中的uid数据存入上下文
	ctx = context.WithValue(ctx, uidKey{}, userid) // uidkey为任意类型
	return handler(ctx, req)
}

// 同一个包中提供获取uid的api
func UserIdFromContext(ctx context.Context) (int64, bool) {
	uid, ok := ctx.Value(uidKey{}).(int64)
	return uid, ok
}

// 日志拦截器
func Logginginterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	cost := time.Since(start)
	fmt.Printf("method=%s code=%s cost=%v\n",
		// 这里 status 来自 google.golang.org/grpc/status；err == nil 时会输出 OK。
		info.FullMethod, status.Code(err), cost)
	return resp, err
}
