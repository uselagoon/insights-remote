package service

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/json"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/tokens"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

const secretTestTokenSecret = "secret"
const secretTestNamespace = "testNS"

func TestWriteRoute(t *testing.T) {
	router := SetupRouter(secretTestTokenSecret)
	w := httptest.NewRecorder()

	token, err := tokens.GenerateTokenForNamespace(secretTestTokenSecret, secretTestNamespace)

	require.NoError(t, err)

	testFacts := internal.Facts{Facts: []internal.Fact{
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
}
