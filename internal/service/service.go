package service

import (
	"fmt"
	"io/ioutil"
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

const deleteFactsType = "direct.delete.facts"
const deleteProblemsType = "direct.delete.problems"

func SetupRouter(secret string, messageQWriter func(data []byte) error, writeToQueue bool) *gin.Engine {
	router := gin.Default()
	r := routerInstance{secret: secret}
	r.MessageQWriter = messageQWriter
	r.WriteToQueue = writeToQueue
	router.POST("/facts", r.writeFacts)
	router.POST("/problems", r.writeProblems)
	router.DELETE("/problems/:source", r.deleteProblems)
	router.DELETE("/problems/:source/:service", r.deleteProblems)
	router.DELETE("/facts/:source", r.deleteFacts)
	router.DELETE("/facts/:source/:service", r.deleteFacts)
	return router
}

func (r *routerInstance) deleteProblems(c *gin.Context) {
	generateDeletionMessage(c, r, deleteProblemsType)
}

func (r *routerInstance) deleteFacts(c *gin.Context) {
	generateDeletionMessage(c, r, deleteFactsType)
}

func generateDeletionMessage(c *gin.Context, r *routerInstance, deletionType string) {
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

	source := c.Params.ByName("source")
	service := c.Params.ByName("service")

	envid, err := strconv.ParseInt(namespace.EnvironmentId, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "BAD REQUEST",
			"message": err.Error(),
		})
		return
	}

	message := DirectDeleteMessage{
		Type:          deletionType,
		EnvironmentId: int(envid),
		Source:        source,
		Service:       service,
	}

	jsonRep, err := json.Marshal(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}

	if err := r.writeToQueue(c, err, jsonRep); err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "okay",
	})
}

func (r *routerInstance) writeProblems(c *gin.Context) {

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

	details := &internal.Problems{Type: "direct.problems"}
	problemList := *new([]internal.Problem)

	if err = c.ShouldBindJSON(&problemList); err != nil {
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
	details.Problems = problemList
	for i := range details.Problems {
		details.Problems[i].EnvironmentId = int(lid)

		if details.Problems[i].Source == "" {
			details.Problems[i].Source = "InsightsRemoteWebService"
		}
	}

	// Write this to the queue

	jsonRep, err := json.Marshal(details)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}

	if err := r.writeToQueue(c, err, jsonRep); err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "okay",
	})
}

func (r *routerInstance) writeToQueue(c *gin.Context, err error, jsonRep []byte) error {
	if r.WriteToQueue {
		err = r.MessageQWriter(jsonRep)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Not writing to queue - would have sent these data %v\n", string(jsonRep))
	}
	return nil
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

	// we try two different ways of parsing incoming facts - first as a simple list of facts
	ByteBody, _ := ioutil.ReadAll(c.Request.Body)

	factList := *new([]internal.Fact)
	if err = json.Unmarshal(ByteBody, &factList); err != nil { // it might just be they're passing the "big" version with all details
		if err = json.Unmarshal(ByteBody, details); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "Unable to parse incoming data",
				"message": err.Error(),
			})
			fmt.Println(err)
			return
		}
	} else {
		details.Facts = factList
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
