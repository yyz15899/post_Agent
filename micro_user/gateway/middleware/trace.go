package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

func TraceingSpan(tracer trace.Tracer) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, span := tracer.Start(c.Request.Context(), c.Request.Method+" "+c.FullPath())
		// 每个接口的spanname不同
		defer span.End()
		// 将包含span的ctx传到后续处理函数
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}

}
