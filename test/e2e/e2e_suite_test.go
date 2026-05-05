/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"fmt"
	"testing"

	"flag"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// Run e2e tests using the Ginkgo runner.
var runIntegration = flag.Bool("e2e", false, "run e2e tests")

func TestE2E(t *testing.T) {
	if !*runIntegration {
		t.Skip("Skipping e2e tests; use -e2e to run them")
	}
	gomega.RegisterFailHandler(ginkgo.Fail)
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Starting insights-remote suite\n")
	ginkgo.RunSpecs(t, "e2e suite")
}
