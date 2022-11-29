package service

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/tokens"
	"net/http"
)

type AuthHeader struct {
	Authorization string `header:"Authorization"`
}

// routerInstance is used to share state
type routerInstance struct {
	secret string
}

func SetupRouter(secret string) *gin.Engine {
	router := gin.Default()
	r := routerInstance{secret: secret}
	router.POST("/facts", r.writeFacts)
	return router
}

func (r *routerInstance) writeFacts(c *gin.Context) {

	// TODO: check incoming packet can is serializable ...

	h := &AuthHeader{}
	if err := c.ShouldBindHeader(&h); err != nil {
		c.JSON(http.StatusOK, err)
	}

	namespace, err := tokens.ValidateAndExtractNamespaceFromToken(r.secret, h.Authorization)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "unauthorized",
			"message": err.Error(),
		})
		return
	}

	fmt.Println("Going to write to namespace ", namespace)

	details := &internal.Facts{}

	if err = c.ShouldBindJSON(details); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "Unable to parse incoming data",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "okay",
	})

}
