package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func Cache() func(c *gin.Context) {
	return func(c *gin.Context) {
		path := c.Request.RequestURI

		// HTML 文件不缓存，确保总是获取最新版本
		if path == "/" || strings.HasSuffix(path, ".html") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		} else if strings.Contains(path, "/assets/") {
			// 带哈希的资源文件（如 index-BrpT8k3A.js）可以长期缓存
			// 因为文件名变化时哈希也会变化，不会有缓存问题
			c.Header("Cache-Control", "public, max-age=31536000, immutable") // one year
		} else {
			// 其他文件短期缓存
			c.Header("Cache-Control", "public, max-age=3600") // one hour
		}
		c.Next()
	}
}
