package middleware

import (
	"github.com/gin-gonic/gin"
)

func InjectMiddleware(key string, dep interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(key, dep)
	}
}
