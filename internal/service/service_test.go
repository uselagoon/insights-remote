package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/json"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/tokens"

	"github.com/stretchr/testify/assert"
)

const secretTestTokenSecret = "secret"
const secretTestNamespace = "testNS"
const testEnvironmentId = "777"

var queueWriterOutput string

func messageQueueWriter(data []byte) error {
	queueWriterOutput = string(data)
	return nil
}

func resetWriterOutput() {
	queueWriterOutput = ""
}

func TestWriteFactsRoute(t *testing.T) {
	defer resetWriterOutput()
	router := SetupRouter(secretTestTokenSecret, messageQueueWriter, true)
	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, tokens.NamespaceDetails{
		Namespace:       secretTestNamespace,
		EnvironmentId:   testEnvironmentId,
		ProjectName:     "Test",
		EnvironmentName: "Test",
	})

	require.NoError(t, err)

	testFacts := internal.Facts{
		Facts: []internal.Fact{
			{
				Name:        "testfact1",
				Value:       "testvalue1",
				Source:      "testsource1",
				Description: "testdescription1",
				Type:        "testtype1",
				Category:    "testcategory1",
				Service:     "testservice1",
			},
		}}
	bodyString, _ := json.Marshal(testFacts)
	req, _ := http.NewRequest(http.MethodPost, "/facts", bytes.NewBuffer(bodyString))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), w.Body.String())
	assert.Contains(t, queueWriterOutput, testFacts.Facts[0].Name)
}

func TestWriteFactsRouteNoProjectData(t *testing.T) {
	defer resetWriterOutput()
	router := SetupRouter(secretTestTokenSecret, messageQueueWriter, true)
	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, tokens.NamespaceDetails{
		Namespace:       secretTestNamespace,
		EnvironmentId:   testEnvironmentId,
		ProjectName:     "Test",
		EnvironmentName: "Test",
	})

	require.NoError(t, err)

	testFacts := []internal.Fact{
		{
			Name:        "testfact1",
			Value:       "testvalue1",
			Source:      "testsource1",
			Description: "testdescription1",
			Type:        "testtype1",
			Category:    "testcategory1",
			Service:     "testservice1",
		},
	}
	bodyString, _ := json.Marshal(testFacts)
	req, _ := http.NewRequest(http.MethodPost, "/facts", bytes.NewBuffer(bodyString))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), w.Body.String())
	assert.Contains(t, queueWriterOutput, testFacts[0].Name)
}

func TestWriteProblemsRoute(t *testing.T) {
	defer resetWriterOutput()
	router := SetupRouter(secretTestTokenSecret, messageQueueWriter, true)
	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, tokens.NamespaceDetails{
		Namespace:       secretTestNamespace,
		EnvironmentId:   testEnvironmentId,
		ProjectName:     "Test",
		EnvironmentName: "Test",
	})

	require.NoError(t, err)

	testProblems := []internal.Problem{
		{
			EnvironmentId:     4,
			Identifier:        "123",
			Version:           "1",
			FixedVersion:      "2",
			Source:            "a unique sources",
			Service:           "test",
			Data:              "test",
			Severity:          "1",
			SeverityScore:     1,
			AssociatedPackage: "test",
			Description:       "test",
			Links:             "test",
		},
	}
	bodyString, _ := json.Marshal(testProblems)
	req, _ := http.NewRequest(http.MethodPost, "/problems", bytes.NewBuffer(bodyString))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	assert.Contains(t, queueWriterOutput, testProblems[0].Source)
}

func TestFactDeletionRoute(t *testing.T) {
	defer resetWriterOutput()
	router := SetupRouter(secretTestTokenSecret, messageQueueWriter, true)
	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, tokens.NamespaceDetails{
		Namespace:       secretTestNamespace,
		EnvironmentId:   testEnvironmentId,
		ProjectName:     "Test",
		EnvironmentName: "Test",
	})

	require.NoError(t, err)

	var bodyString []byte
	req, _ := http.NewRequest(http.MethodDelete, "/facts/testsource", bytes.NewBuffer(bodyString))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProblemDeletionRoute(t *testing.T) {
	defer resetWriterOutput()
	router := SetupRouter(secretTestTokenSecret, messageQueueWriter, true)
	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, tokens.NamespaceDetails{
		Namespace:       secretTestNamespace,
		EnvironmentId:   testEnvironmentId,
		ProjectName:     "Test",
		EnvironmentName: "Test",
	})

	require.NoError(t, err)

	var bodyString []byte
	req, _ := http.NewRequest(http.MethodDelete, "/problems/testsource", bytes.NewBuffer(bodyString))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHeaderMiddleware(t *testing.T) {
	defer resetWriterOutput()
	router := SetupRouter(secretTestTokenSecret, messageQueueWriter, true)

	router.POST("/getnamespacetest", func(c *gin.Context) {
		val, exists := c.Get("namespace")
		if !exists {
			c.XML(http.StatusBadRequest, "failed")
		}
		c.XML(http.StatusOK, val)
	})

	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, tokens.NamespaceDetails{
		Namespace:       secretTestNamespace,
		EnvironmentId:   testEnvironmentId,
		ProjectName:     "Test",
		EnvironmentName: "Test",
	})

	require.NoError(t, err)

	// First, let's test a failure

	var bodyString []byte
	req, _ := http.NewRequest(http.MethodPost, "/getnamespacetest", bytes.NewBuffer(bodyString))
	req.Header.Set("Authorization", "busted")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Now we get success
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/getnamespacetest", bytes.NewBuffer(bodyString))
	req2.Header.Set("Authorization", token)
	req2.Header.Set("Content-Type", "application/xml")
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), secretTestNamespace)

}
