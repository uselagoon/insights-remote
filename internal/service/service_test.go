package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/json"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/tokens"

	"github.com/stretchr/testify/assert"
)

const secretTestTokenSecret = "secret"
const secretTestNamespace = "testNS"

var queueWriterOutput string

func messageQueueWriter(data []byte) error {
	//fmt.Println(string(data))
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
		EnvironmentId:   "1",
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

func TestWriteProblemsRoute(t *testing.T) {
	defer resetWriterOutput()
	router := SetupRouter(secretTestTokenSecret, messageQueueWriter, true)
	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, tokens.NamespaceDetails{
		Namespace:       secretTestNamespace,
		EnvironmentId:   "1",
		ProjectName:     "Test",
		EnvironmentName: "Test",
	})

	require.NoError(t, err)

	testFacts := internal.Problems{
		Problems: []internal.Problem{
			{
				EnvironmentId:     "1",
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
		}}
	bodyString, _ := json.Marshal(testFacts)
	req, _ := http.NewRequest(http.MethodPost, "/problems", bytes.NewBuffer(bodyString))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), w.Body.String())
	assert.Contains(t, queueWriterOutput, testFacts.Problems[0].Source)
}
