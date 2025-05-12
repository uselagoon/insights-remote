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
	"lagoon.sh/insights-remote/test/utils"
	"os/exec"
	"strings"
	"testing"
	"time"
)
import . "github.com/onsi/ginkgo/v2"
import . "github.com/onsi/gomega"

const (
	clustername = "insights-remote"
	namespace   = "lagoon"
	timeout     = "600s"
)

var testNamespaces = []string{
	"test-lagoon-remote",
	"test-ns1",
}

// We want to write the following ginko tests
// Test that the namespaces get tokens injected
// Test that we can write to the lagoon insights queue
// ... that's all for now
func TestMyPackage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kind Suite")
}

var _ = BeforeSuite(func() {
	err := CheckKindSetup()
	if err != nil {
		Fail(fmt.Sprintf("Error checking kind setup: %v", err))
	}
	// here we want to build and push the image to the kind cluster
	// Build and push the image to the kind cluster
	if output, err := utils.Run(exec.Command("make", "docker-build")); err != nil {
		Fail(fmt.Sprintf("Error building image: %v", err))
	} else {
		fmt.Printf("Output: %s\n", string(output))
	}

	if output, err := utils.Run(exec.Command("kind", "load", "docker-image", "controller:latest", "--name", clustername)); err != nil {
		Fail(fmt.Sprintf("Error loading image: %v", err))
	} else {
		fmt.Printf("Output: %s\n", string(output))
	}

	// let's create the namespaces
	for _, ns := range testNamespaces {
		output, err := runKubectlCommand("create", "namespace", ns)
		if err != nil {
			Fail(fmt.Sprintf("error creating namespace %s: %v", ns, err))
		}
		fmt.Println(output)
	}

	// Create the controller
	out, err := runKubectlCommand("apply", "-f", "test/e2e/assets/controller_deployment.yaml")
	if err != nil {
		Fail(fmt.Sprintf("Error creating controller: %v", err))
	}
	fmt.Printf("Output: %s\n", out)
})

var _ = Describe("Controller", Ordered, func() {

	BeforeAll(func() {
		fmt.Println("Setting up kind cluster")
		// Create the namespace
		err := CheckKindSetup()
		if err != nil {
			Fail(fmt.Sprintf("Error checking kind setup: %v", err))
		}
		err = ClearTestData()
		if err != nil {
			fmt.Printf("Error clearing test data: %v", err)
		}

		// let's create the namespaces
		for _, ns := range testNamespaces {
			output, err := runKubectlCommand("create", "namespace", ns)
			if err != nil {
				Fail(fmt.Sprintf("error creating namespace %s: %v", ns, err))
			}
			fmt.Println(output)
		}

		// Create the controller
		_, err = runKubectlCommand("apply", "-f", "test/e2e/assets/nstest.yaml")
		if err != nil {
			Fail(fmt.Sprintf("Error creating controller: %v", err))
		}
		// Create the secrets
		// Create the configmap
	})

	It("should add a token secret to a namespace with the appropriate labels", func() {
		runKubectlCommand("apply", "-f", "test/e2e/assets/nstest.yaml")
		//defer runKubectlCommand("delete", "-f", "test/e2e/assets/nstest.yaml") // clean up

		//Expect(true).To(Equal(true)) // TODO: check that the token secret is created
		//
		// let's wait a little bit for the token to be created
		time.Sleep(5 * time.Second)
		out, err := runKubectlCommand("get", "secrets", "-n", "test-lagoon-ns")
		if err != nil {
			Fail(fmt.Sprintf("Error getting secrets: %v", err))
		}
		if !strings.Contains(out, "insights-token") {
			Fail("Token secret not found")
		}
	})
})

func CheckKindSetup() error {
	// check if there is an "insights-remote" cluster
	cmd := exec.Command("kind", "get", "clusters")
	outBytes, err := utils.Run(cmd)
	if err != nil {
		return err
	}
	out := string(outBytes)
	if !strings.Contains(out, clustername) {
		return fmt.Errorf("cluster %s not found", clustername)
	}
	return nil
}

func ClearTestData() error {
	for _, ns := range testNamespaces {
		output, err := runKubectlCommand("delete", "namespace", ns)
		if err != nil {
			return fmt.Errorf("error deleting namespace %s: %v", ns, err)
		}
		fmt.Println(output)
	}
	return nil
}

func runKubectlCommand(args ...string) (string, error) {
	// ensure we set the context
	args = append(args, "--context", fmt.Sprintf("kind-%v", clustername))
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output), nil
}
func phraseExistsInLogs(deploymentName, namespace, phrase string) (bool, error) {
	time.Sleep(5 * time.Second) // wait for the deployment to be ready
	out, err := runKubectlCommand("logs", "-l", fmt.Sprintf("app=%s", deploymentName), "-n", namespace)
	if err != nil {
		return false, fmt.Errorf("error getting logs for deployment %s: %v", deploymentName, err)
	}
	if !strings.Contains(out, phrase) {
		return false, nil //not an error, just not in the logs
	}
	return true, nil
}
