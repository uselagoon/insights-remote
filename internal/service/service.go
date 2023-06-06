package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/util/json"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/tokens"
)

type AuthHeader struct {
	Authorization string `header:"Authorization"`
}

// routerInstance is used to share state
type routerInstance struct {
	secret         string
	MessageQWriter func(data []byte) error
	WriteToQueue   bool
}

func SetupRouter(secret string, messageQWriter func(data []byte) error, writeToQueue bool) *gin.Engine {
	router := gin.Default()
	r := routerInstance{secret: secret}
	r.MessageQWriter = messageQWriter
	r.WriteToQueue = writeToQueue
	router.POST("/facts", r.writeFacts)
	return router
}

func (r *routerInstance) writeFacts(c *gin.Context) {

	h := &AuthHeader{}
	if err := c.ShouldBindHeader(&h); err != nil {
		c.JSON(http.StatusOK, err)
	}

	namespace, err := tokens.ValidateAndExtractNamespaceDetailsFromToken(r.secret, h.Authorization)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "unauthorized",
			"message": err.Error(),
		})
		return
	}

	fmt.Println("Going to write to namespace ", namespace)

	//TODO: drop "InsightsType" for Type of the form "direct.fact"/"direct.problem"
	details := &internal.Facts{Type: "direct.facts"}

	if err = c.ShouldBindJSON(details); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "Unable to parse incoming data",
			"message": err.Error(),
		})
		fmt.Println(err)
		return
	}

	//let's force our facts to get pushed to the right place
	lid, err := strconv.ParseInt(namespace.EnvironmentId, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "Unable to parse environment ID",
			"message": err.Error(),
		})
		fmt.Println(err)
		return
	}

	details.EnvironmentId = int(lid)
	details.ProjectName = namespace.ProjectName
	details.EnvironmentName = namespace.EnvironmentName
	for i := range details.Facts {
		details.Facts[i].EnvironmentId = namespace.EnvironmentId
		details.Facts[i].EnvironmentName = namespace.EnvironmentName
		details.Facts[i].ProjectName = namespace.ProjectName
		if details.Facts[i].Source == "" {
			details.Facts[i].Source = "InsightsRemoteWebService"
		}
	}

	// Write this to the queue

	jsonRep, err := json.Marshal(details)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}

	if r.WriteToQueue {
		err = r.MessageQWriter(jsonRep)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err)
			return
		}
	} else {
		fmt.Printf("Not writing to queue - would have sent these data %v\n", string(jsonRep))
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "okay",
	})
}