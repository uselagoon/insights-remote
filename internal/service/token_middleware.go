package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"lagoon.sh/insights-remote/internal/tokens"
)

// TokenParserMiddleware is a context aware middleware
func TokenParserMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := &AuthHeader{}
		err := c.ShouldBindHeader(&h)
		if err != nil {
			c.JSON(http.StatusBadRequest, "Missing bearer token")
			c.Abort()
			return
		}

		namespace, err := tokens.ValidateAndExtractNamespaceDetailsFromToken(secret, h.Authorization)

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "unauthorized",
				"message": err.Error(),
			})
			c.Abort()
			return
		}
		c.Set("namespace", namespace)
		c.Next()
	}
}

// GetNamespaceDetails retrieves the authenticated namespace from the gin context.
// It should only be called in handlers protected by TokenParserMiddleware.
func GetNamespaceDetails(c *gin.Context) (tokens.NamespaceDetails, bool) {
	n, exists := c.Get("namespace")
	if !exists {
		return tokens.NamespaceDetails{}, false
	}
	namespace, ok := n.(tokens.NamespaceDetails)
	return namespace, ok
}
