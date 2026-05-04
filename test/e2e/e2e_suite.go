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
	"os"
	"os/exec"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"lagoon.sh/insights-remote/test/utils"
)

const (
	namespace = "insights-remote-system"
	timeout   = "600s"
)

func init() {
	kindIP = os.Getenv("KIND_NODE_IP")
}

var (
	// duration = 600 * time.Second
	// interval = 1 * time.Second

	kindIP string

	metricLabels = []string{
		"controller_runtime_active_workers",
	}
)

var _ = ginkgo.Describe("controller", ginkgo.Ordered, func() {
	ginkgo.BeforeAll(func() {
		ginkgo.By("start local services")
		gomega.Expect(utils.StartLocalServices()).To(gomega.Succeed())

		ginkgo.By("creating manager namespace")
		cmd := exec.Command(utils.Kubectl(), "create", "ns", namespace)
		_, _ = utils.Run(cmd)

		ginkgo.By("create broker secret")
		cmd = exec.Command(utils.Kubectl(), "-n", namespace, "create", "secret", "generic", "lagoon-broker-tls",
			"--from-file=tls.crt=local-dev/certificates/clienttls.crt",
			"--from-file=tls.key=local-dev/certificates/clienttls.key",
			"--from-file=ca.crt=local-dev/certificates/ca.crt")
		_, _ = utils.Run(cmd)

		// when running a re-test, it is best to make sure the old namespace doesn't exist
		ginkgo.By("removing existing test resources")

		// remove the example namespace
		cmd = exec.Command(utils.Kubectl(), "delete", "ns", "example-nginx")
		_, _ = utils.Run(cmd)
	})

	// comment to prevent cleaning up controller namespace and local services
	ginkgo.AfterAll(func() {
		ginkgo.By("stop metrics consumer")
		utils.StopMetricsConsumer()

		// remove the example namespace
		cmd := exec.Command(utils.Kubectl(), "delete", "ns", "example-nginx")
		_, _ = utils.Run(cmd)

		ginkgo.By("removing manager namespace")
		cmd = exec.Command(utils.Kubectl(), "delete", "ns", namespace)
		_, _ = utils.Run(cmd)

		ginkgo.By("stop local services")
		utils.StopLocalServices()
	})

	ginkgo.Context("Operator", func() {
		ginkgo.It("should run successfully", func() {
			// start tests
			var controllerPodName string
			var err error

			// projectimage stores the name of the image used in the example
			var projectimage = "example.com/insights-remote:v0.0.1"

			ginkgo.By("building the manager(Operator) image")
			cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectimage))
			_, err = utils.Run(cmd)
			gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())

			ginkgo.By("loading the the manager(Operator) image on Kind")
			err = utils.LoadImageToKindClusterWithName(projectimage)
			gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())

			ginkgo.By("deploying the controller-manager")
			cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectimage))
			_, err = utils.Run(cmd)
			gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())

			ginkgo.By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func() error {
				// Get pod name

				cmd = exec.Command(utils.Kubectl(), "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
				podNames := utils.GetNonEmptyLines(string(podOutput))
				if len(podNames) != 1 {
					return fmt.Errorf("expect 1 controller pods running, but got %d", len(podNames))
				}
				controllerPodName = podNames[0]
				gomega.ExpectWithOffset(2, controllerPodName).Should(gomega.ContainSubstring("controller-manager"))

				cmd = exec.Command(utils.Kubectl(), "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				status, err := utils.Run(cmd)
				gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
				if string(status) != "Running" {
					return fmt.Errorf("controller pod in %s status", status)
				}
				return nil
			}
			gomega.EventuallyWithOffset(1, verifyControllerUp, time.Minute, time.Second).Should(gomega.Succeed())

			time.Sleep(5 * time.Second)

			ginkgo.By("start metrics consumer")
			gomega.Expect(utils.StartMetricsConsumer()).To(gomega.Succeed())

			time.Sleep(30 * time.Second)

			// insert tests here

			ginkgo.By("validating that unauthenticated metrics requests fail")
			runCmd := `curl -s -k https://insights-remote-controller-manager-metrics-service.insights-remote-system.svc.cluster.local:8443/metrics | grep -v "#" | grep "controller_"`
			_, err = utils.RunCommonsCommand(namespace, runCmd)
			gomega.ExpectWithOffset(2, err).To(gomega.HaveOccurred())

			ginkgo.By("validating that authenticated metrics requests succeed with metrics")
			runCmd = `curl -s -k -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" https://insights-remote-controller-manager-metrics-service.insights-remote-system.svc.cluster.local:8443/metrics | grep -v "#" | grep "controller_"`
			output, err := utils.RunCommonsCommand(namespace, runCmd)
			gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
			fmt.Printf("metrics: %s", string(output))
			err = utils.CheckStringContainsStrings(string(output), metricLabels)
			gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())

			// verify that default HTTP response code is 404
			ginkgo.By("validating default HTTP response code")
			runCmd = fmt.Sprintf(`curl -s -I http://non-existing-domain.%s.nip.io/`, kindIP)
			output, _ = utils.RunCommonsCommand(namespace, runCmd)
			fmt.Printf("curl: %s", string(output))
			err = utils.CheckStringContainsStrings(string(output), []string{"404 Not Found"})
			gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
			// End tests
		})
	})
})
