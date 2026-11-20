package middleware

import (
	"context"
	"time"
)

func VoteTiming() context.Context {
	// 创建2s上下文
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return ctx
}
