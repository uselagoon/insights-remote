package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cri-api/pkg/errors"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	TmpDir          = "/tmp"
	SbomOutput      = "cyclonedx-json"
	ImageInspectCmd = "skopeo"
	defaultTimeout  = 5 * time.Minute
)

func RunGetImageList(mgr manager.Manager, namespace string) ([]string, error) {
	ctx := context.TODO()

	deploymentList := &appsv1.DeploymentList{}
	err := mgr.GetClient().List(ctx, deploymentList, client.InNamespace(namespace))
	if err != nil {
		if errors.IsNotFound(err) {
			log.Fatalf("Namespace not found: %s", namespace)
		} else {
			log.Fatal(err)
		}
	}

	images := make(map[string]bool)
	for _, deployment := range deploymentList.Items {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			images[container.Image] = true
		}
	}

	var uniqueImages []string
	for image := range images {
		uniqueImages = append(uniqueImages, image)
	}

	return uniqueImages, nil
}

func ExtractImageName(image string) (string, error) {
	parts := strings.Split(image, "@")

	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected result format: %s", image)
	}

	imageName := parts[0]

	return imageName, nil
}

func RunImageInspect(imageFull, outputFilePath string) error {
	cmd := exec.Command(ImageInspectCmd, "--retry-times", "5", "docker://"+imageFull, "--tls-verify=false")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	file, err := os.Create(outputFilePath)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd.Stdout = file

	err = cmd.Run()
	if err != nil {
		return err
	}

	fmt.Println("Successfully ran image inspection")
	return nil
}

func RunSbomScanInPod(client client.Client, images []string, namespace string, buildName string, project string, environment string) error {
	fmt.Println("Running sbom scan using syft")

	// TEST
	// images = []string{"harbor.test6.amazee.io/magento2-example-simple/main/php@sha256:fc99ea8f795ec9509541808f555151c4937602bd59cc5701735e06bc120c7f69"}
	// project = "magento2-example-simple"
	// environment = "main"
	// buildName = "lagoon-build-5x9c4t"
	// namespace = "magento2-example-simple-main"

	// fmt.Println(images)
	// fmt.Println(namespace)
	// fmt.Println(project)
	// fmt.Println(environment)
	// fmt.Println(buildName)

	tmpDir := "/tmp"

	harborAdmin := os.Getenv("HARBOR_ADMIN")
	if harborAdmin == "" {
		harborAdmin = "admin"
	}

	for _, image := range images {
		imageName, err := ExtractImageName(image)
		if err != nil {
			fmt.Printf("Error extracting image name: %v\n", err)
		}

		// check if image is in docker-host first
		imageExistsOnDockerHost, err := checkIfImageExistsInDockerHost(image)
		if err != nil {
			fmt.Printf("Error checking docker-host for image: %v\n", err)
		}

		// @TODO need a better way to get service/image name here
		service := path.Base(imageName)
		// fmt.Println(service)

		terminationGracePeriodSeconds := int64(600)

		// Create a Pod spec
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "insights-runner-" + service,
				Namespace:    namespace,
				Labels: map[string]string{
					"lagoon.sh/buildName": buildName,
				},
			},
			Spec: v1.PodSpec{
				ServiceAccountName:            "lagoon-deployer",
				TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
				Containers: []v1.Container{
					{
						Name:  "syft-and-skopeo-runner",
						Image: "imagecache.amazeeio.cloud/uselagoon/build-deploy-image:edge",
						Command: []string{
							"/bin/sh",
							"-c",
							fmt.Sprintf(`
TMP_DIR="%s"
IMAGE_EXISTS_ON_DOCKER_HOST="%v"
IMAGE_FULL="%s"
SERVICE="%s"
NAMESPACE="%s"
PROJECT="%s"
ENVIRONMENT="%s"
LAGOON_BUILD_NAME="%s"

SBOM_OUTPUT="cyclonedx-json"
SBOM_OUTPUT_FILE="${TMP_DIR}/${IMAGE_FULL}.cyclonedx.json.gz"
SBOM_CONFIGMAP="lagoon-insights-sbom-${SERVICE}"
IMAGE_INSPECT_CONFIGMAP="lagoon-insights-image-${SERVICE}"
IMAGE_INSPECT_OUTPUT_FILE="${TMP_DIR}/${IMAGE_FULL}.image-inspect.json.gz"


echo "TMP_DIR=\"$TMP_DIR\""
echo "IMAGE_EXISTS_ON_DOCKER_HOST=\"$IMAGE_EXISTS_ON_DOCKER_HOST\""
echo "IMAGE_FULL=\"$IMAGE_FULL\""
echo "SERVICE=\"$SERVICE\""
echo "NAMESPACE=\"$NAMESPACE\""
echo "PROJECT=\"$PROJECT\""
echo "ENVIRONMENT=\"$ENVIRONMENT\""
echo "LAGOON_BUILD_NAME=\"$LAGOON_BUILD_NAME\""

echo "SBOM_OUTPUT=\"cyclonedx-json\""
echo "SBOM_OUTPUT_FILE=\"$TMP_DIR/$IMAGE_FULL.cyclonedx.json.gz\""
echo "SBOM_CONFIGMAP=\"lagoon-insights-sbom-$SERVICE\""
echo "IMAGE_INSPECT_CONFIGMAP=\"lagoon-insights-image-$SERVICE\""
echo "IMAGE_INSPECT_OUTPUT_FILE=\"$TMP_DIR/$IMAGE_FULL.image-inspect.json.gz\""

DOCKER_CONFIG=/config

# Extract username and password from the DOCKER_CONFIG_CONTENT variable
harbor_username=$(echo $DOCKER_CONFIG_CONTENT | jq -r '.auths["harbor.test6.amazee.io"].username')
harbor_password=$(echo $DOCKER_CONFIG_CONTENT | jq -r '.auths["harbor.test6.amazee.io"].password')

export HARBOR_ADMIN="$harbor_username"
export HARBOR_ADMIN_PASSWORD="$harbor_password"

set +x

runSkopeoInspect() {
  echo "Running image inspect on: ${IMAGE_FULL}"

  # Check if the directory exists, create it if not
  IMAGE_INSPECT_DIR=$(dirname "$IMAGE_INSPECT_OUTPUT_FILE")
  if [ ! -d "$IMAGE_INSPECT_DIR" ]; then
    mkdir -p "$IMAGE_INSPECT_DIR"
    echo "Directory $IMAGE_INSPECT_DIR created successfully"
  fi

	if "$IMAGE_EXISTS_ON_DOCKER_HOST"; then
		echo "running against docker-daemon"
		skopeo_output=$(skopeo inspect --daemon-host=http://docker-host.lagoon.svc.cluster.local:2375 --retry-times 5 docker-daemon:${IMAGE_FULL}:latest --tls-verify=false | gzip > ${IMAGE_INSPECT_OUTPUT_FILE})
	else
		echo "running skopeo against registry"
		skopeo_output=$(skopeo inspect --creds=${HARBOR_ADMIN}:${HARBOR_ADMIN_PASSWORD} --retry-times 5 docker://${IMAGE_FULL} --tls-verify=false | gzip > ${IMAGE_INSPECT_OUTPUT_FILE})
	fi

	if [ $? -eq 0 ]; then
    echo "Image inspection successful."
  else
    echo "Error: Skopeo command failed. Skipping image inspection..."
    return 1
  fi
}

# Function to process image inspection
processImageInspect() {
  # Check if the skopeo command was successful before proceeding
  if [ $? -ne 0 ]; then
    echo "Error: Skopeo command failed. Skipping image inspection..."
    return
  fi

  # If lagoon-insights-image-inpsect-[IMAGE] configmap already exists then we need to update, else create new
  # if kubectl -n ${NAMESPACE} get configmap $IMAGE_INSPECT_CONFIGMAP &> /dev/null; then
  #     kubectl \
  #         -n ${NAMESPACE} \
  #         create configmap $IMAGE_INSPECT_CONFIGMAP \
  #         --from-file=${IMAGE_INSPECT_OUTPUT_FILE} \
  #         -o json \
  #         --dry-run=client | kubectl replace -f -
  # else
  #     kubectl \
  #         -n ${NAMESPACE} \
  #         create configmap ${IMAGE_INSPECT_CONFIGMAP} \
  #         --from-file=${IMAGE_INSPECT_OUTPUT_FILE}
  # fi
  # kubectl \
  #     -n ${NAMESPACE} \
  #     label configmap ${IMAGE_INSPECT_CONFIGMAP} \
  #     lagoon.sh/insightsProcessed- \
  #     lagoon.sh/insightsType=image-gz \
  #     lagoon.sh/buildName=${LAGOON_BUILD_NAME} \
  #     lagoon.sh/project=${PROJECT} \
  #     lagoon.sh/environment=${ENVIRONMENT} \
  #     lagoon.sh/service=${IMAGE_NAME}

  echo "Image inspection completed successfully."
}

# Function to run syft packages command
runSyftPackages() {
  echo "Running sbom scan using syft: ${IMAGE_FULL}"

  SBOM_OUTPUT_DIR=$(dirname "$SBOM_OUTPUT_FILE")
  if [ ! -d "$SBOM_OUTPUT_DIR" ]; then
    mkdir -p "$SBOM_OUTPUT_DIR"
    echo "Directory $SBOM_OUTPUT_DIR created successfully"
  fi

  # Check if the SBOM_OUTPUT_FILE exists, create it if not
  if [ ! -f "$SBOM_OUTPUT_FILE" ]; then
    touch "$SBOM_OUTPUT_FILE"
    echo "File $SBOM_OUTPUT_FILE created successfully"
  fi

  if "$IMAGE_EXISTS_ON_DOCKER_HOST"; then
  	echo "running against docker-daemon"
	syft_output=$(DOCKER_HOST=tcp://docker-host.lagoon.svc.cluster.local:2375 docker run --rm -v /var/run/docker.sock:/var/run/docker.sock imagecache.amazeeio.cloud/anchore/syft packages ${IMAGE_FULL} --quiet -o ${SBOM_OUTPUT} | gzip > ${SBOM_OUTPUT_FILE})

  else
	echo "running against registry"
	syft_output=$(DOCKER_HOST=tcp://docker-host.lagoon.svc.cluster.local:2375 docker run --rm -v $DOCKER_CONFIG:/config -v /var/run/docker.sock:/var/run/docker.sock imagecache.amazeeio.cloud/anchore/syft packages ${IMAGE_FULL} --quiet -o ${SBOM_OUTPUT} | gzip > ${SBOM_OUTPUT_FILE})
  fi

  # Check the exit status of the syft command
  if [ $? -eq 0 ]; then
		return 0
  else
    echo "Error: Syft command failed. Skipping SBOM processing..."
    return 1
  fi
}

# Function to process SBOM
processSbom() {
  FILESIZE=$(du -b "$SBOM_OUTPUT_FILE" | cut -f1)
  echo "Size of ${SBOM_OUTPUT_FILE} = $FILESIZE bytes."

  if [ "$FILESIZE" -gt 950000 ]; then
    echo "$SBOM_OUTPUT_FILE is too large, skipping pushing to configmap"
    return
  else
    if [ $? -ne 0 ]; then
      echo "Error: Syft command failed. Skipping SBOM processing..."
      return
    fi

    # if kubectl -n ${NAMESPACE} get configmap $SBOM_CONFIGMAP &> /dev/null; then
    #   kubectl \
    #     -n ${NAMESPACE} \
    #     create configmap $SBOM_CONFIGMAP \
    #     --from-file=${SBOM_OUTPUT_FILE} \
    #     -o json \
    #     --dry-run=client | kubectl replace -f -
    # else
    #   kubectl \
    #     -n ${NAMESPACE} \
    #     create configmap ${SBOM_CONFIGMAP} \
    #     --from-file=${SBOM_OUTPUT_FILE}
    # fi

    # kubectl \
    #   -n ${NAMESPACE} \
    #   label configmap ${SBOM_CONFIGMAP} \
    #   lagoon.sh/insightsProcessed- \
    #   lagoon.sh/insightsType=sbom-gz \
    #   lagoon.sh/buildName=${LAGOON_BUILD_NAME} \
    #   lagoon.sh/project=${PROJECT} \
    #   lagoon.sh/environment=${ENVIRONMENT} \
    #   lagoon.sh/service=${SERVICE}

    echo "Successfully generated SBOM for ${IMAGE_FULL}"
  fi
}

runSkopeoInspect
processImageInspect

if runSyftPackages; then
  processSbom
fi
`,
								tmpDir, imageExistsOnDockerHost, imageName, service, namespace, project, environment, buildName),
						},
						Env: []v1.EnvVar{
							{
								Name:  "HARBOR_ADMIN",
								Value: harborAdmin,
							},
							{
								Name: "DOCKER_CONFIG_CONTENT",
								ValueFrom: &v1.EnvVarSource{
									SecretKeyRef: &v1.SecretKeySelector{
										LocalObjectReference: v1.LocalObjectReference{
											Name: "lagoon-internal-registry-secret",
										},
										Key: ".dockerconfigjson",
									},
								},
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      "docker-sock",
								MountPath: "/var/run/docker.sock",
							},
							{
								Name:      "docker-config",
								MountPath: "/config",
							},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "docker-sock",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/var/run/docker.sock",
							},
						},
					},
					{
						Name: "docker-config",
						VolumeSource: v1.VolumeSource{
							Secret: &v1.SecretVolumeSource{
								SecretName: "lagoon-internal-registry-secret",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
			},
		}

		// Create the Pod
		err = client.Create(context.Background(), pod)
		if err != nil {
			klog.Fatalf("Failed to create Pod: %v", err)
		}

		// Wait for the Pod to be running
		err = waitForPodRunning(client, namespace, pod.Name, defaultTimeout)
		if err != nil {
			klog.Fatalf("Failed to wait for Pod to be running: %v", err)
		}

		fmt.Printf("Pod %s finished running\n", pod.Name)
	}
	return nil
}

func checkIfImageExistsInDockerHost(imageName string) (bool, error) {
	cmd := exec.Command("DOCKER_HOST=tcp://docker-host.lagoon.svc.cluster.local:2375", "docker", "images")
	grepCmd := exec.Command("grep", imageName)

	grepCmd.Stdin, _ = cmd.StdoutPipe()

	output, err := grepCmd.Output()
	if err != nil {
		fmt.Printf("Cannot find image on Docker host: %v\n", err)
		return false, err
	}

	if len(output) > 0 {
		return false, nil
	} else {
		return true, nil
	}
}

// waitForPodRunning waits for the specified Pod to be in the Running phase.
func waitForPodRunning(c client.Client, namespace, podName string, timeout time.Duration) error {
	return wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		pod := &v1.Pod{}
		err := c.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: podName}, pod)
		if err != nil {
			return false, fmt.Errorf("failed to get Pod %s: %w", podName, err)
		}
		if pod.Status.Phase == v1.PodRunning {
			return true, nil
		}
		return false, nil
	})
}

func RunSbomScan(images []string, namespace string, buildName string, project string, environment string) error {
	fmt.Println("Running sbom scan using syft")

	dockerHost := os.Getenv("DOCKER_HOST_PORT")
	if dockerHost == "" {
		dockerHost = "tcp://docker-host.lagoon.svc:2375"
	}

	//
	// TEST LOCALLY
	// images = []string{"harbor.test6.amazee.io/magento2-example-simple/main/php@sha256:fc99ea8f795ec9509541808f555151c4937602bd59cc5701735e06bc120c7f69"}
	// project = "magento2-example-simple"
	// environment = "main"
	// buildName = "lagoon-build-5x9c4t"
	// namespace = "magento2-example-simple-main"
	//
	//

	for _, image := range images {
		imageName, err := ExtractImageName(image)
		if err != nil {
			fmt.Printf("Error extracting image name: %v\n", err)
		}

		// check if image exists
		exists, err := ImageExists(imageName)
		if err != nil {
			log.Printf("Error checking if image %s exists: %v\n", imageName, err)
			continue
		}

		if !exists {
			log.Printf("Image %s does not exist. Skipping scan.\n", imageName)
			continue
		}

		sbomOutputFile := fmt.Sprintf("/tmp/%s.cyclonedx.json", imageName)

		// sbomScanCmd := fmt.Sprintf("docker run --rm -v /var/run/docker.sock:/var/run/docker.sock imagecache.amazeeio.cloud/anchore/syft packages %s -o cyclonedx-json", imageName)
		sbomScanCmd := fmt.Sprintf("DOCKER_HOST=%s docker run --rm -v /var/run/docker.sock:/var/run/docker.sock imagecache.amazeeio.cloud/anchore/syft packages %s -o cyclonedx-json", dockerHost, imageName)

		fmt.Println(sbomScanCmd)

		cmd := exec.Command("sh", "-c", sbomScanCmd)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err != nil {
			fmt.Printf("Error running SBOM scan for image %s: %v\n", imageName, err)
			fmt.Println("SBOM scan output (stderr):", stderr.String())
		}

		result := stdout.String()

		// fmt.Println("SBOM scan output (stdout):", result)

		outputDir := filepath.Dir(sbomOutputFile)
		err = os.MkdirAll(outputDir, 0755)
		if err != nil {
			fmt.Printf("Error creating directory for SBOM output file: %v\n", err)
			return err
		}

		file, err := os.Create(sbomOutputFile)
		if err != nil {
			fmt.Printf("Error creating SBOM output file: %v\n", err)
			return err
		}
		defer file.Close()

		_, err = file.WriteString(result)
		if err != nil {
			fmt.Printf("Error writing SBOM result to file: %v\n", err)
			return err
		}

		sbomOutputFileGz := sbomOutputFile + ".gz"
		err = gzipFile(sbomOutputFile, sbomOutputFileGz)
		if err != nil {
			fmt.Printf("Error compressing SBOM output file: %v\n", err)
		}

		fileSize, err := getFileSize(sbomOutputFile + ".gz")
		if err != nil {
			fmt.Printf("Error getting file size: %v\n", err)
		}
		fmt.Printf("Size of %s = %d bytes\n", sbomOutputFileGz, fileSize)

		if fileSize > 950000 {
			_, err := fmt.Printf("%s is too large, skipping pushing to configmap\n", sbomOutputFileGz)
			return err
		}

		fmt.Printf("Successfully generated SBOM for %s\n", imageName)

		// @TODO need a better way to get service/image name here
		service := path.Base(imageName)
		fmt.Println(service)

		sbomConfigmap := "lagoon-insights-sbom-" + strings.ReplaceAll(service, "/", "-")
		fmt.Println(sbomConfigmap)

		// Update or create the configmap
		if _, err := os.Stat(sbomOutputFileGz); err == nil {
			// Check if the configmap already exists
			getCmd := exec.Command("kubectl", "-n", namespace, "get", "configmap", sbomConfigmap)
			getOutput, err := getCmd.CombinedOutput()
			if err == nil {
				fmt.Println("Configmap already exists:", string(getOutput))

				// Update the existing configmap
				fmt.Println("kubectl", "-n", namespace, "create", "configmap", sbomConfigmap,
					"--from-file="+sbomOutputFileGz,
					"-o", "json",
					"--dry-run=client")
				updateCmd := exec.Command("kubectl", "-n", namespace, "create", "configmap", sbomConfigmap,
					"--from-file="+sbomOutputFileGz,
					"-o", "json",
					"--dry-run=client")
				updateCmdOutput, err := updateCmd.Output()
				if err != nil {
					fmt.Printf("Error creating configmap: %v\n", err)
					fmt.Println("kubectl create output (stderr):", err.Error())
					return err
				}

				replaceCmd := exec.Command("kubectl", "-n", namespace, "replace", "-f", "-")
				replaceCmd.Stdin = bytes.NewReader(updateCmdOutput)
				replaceCmdOutput, err := replaceCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Error updating configmap: %v\n", err)
					fmt.Println("kubectl replace output (stderr):", string(replaceCmdOutput))
					return err
				}
				fmt.Println("Configmap updated successfully")
			} else {
				// Create a new configmap
				createCmd := exec.Command("kubectl", "-n", namespace, "create", "configmap", sbomConfigmap,
					"--from-file="+sbomOutputFileGz,
				)
				createOutput, err := createCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Error creating configmap: %v\n", err)
					fmt.Println("kubectl create output:", string(createOutput))
					return err
				}
				fmt.Println("Configmap created successfully")
			}
		} else {
			fmt.Printf("SBOM output file not found: %s\n", sbomOutputFileGz)
			return err
		}

		// Label the configmap
		labelCmd := exec.Command("kubectl", "-n", namespace, "label", "configmap", sbomConfigmap,
			"lagoon.sh/insightsProcessed-",
			"lagoon.sh/insightsType=sbom-gz",
			"lagoon.sh/buildName="+buildName,
			"lagoon.sh/project="+project,
			"lagoon.sh/environment="+environment,
			"lagoon.sh/service="+service,
		)
		labelOutput, err := labelCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error labeling configmap: %v\n", err)
			fmt.Println("kubectl label output (stderr):", string(labelOutput))
			return err
		}
		fmt.Println("Configmap labeled successfully")

	}

	return nil
}

func ImageExists(image string) (bool, error) {
	dockerHost := os.Getenv("DOCKER_HOST_PORT")
	if dockerHost == "" {
		dockerHost = "tcp://docker-host.lagoon.svc.cluster.local:2375"
	}
	dockerCmd := exec.Command("docker", "image", "inspect", image)
	dockerCmd.Env = append(os.Environ(), fmt.Sprintf("DOCKER_HOST=%s", dockerHost))

	output, err := dockerCmd.CombinedOutput()

	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ExitCode() == 0 {
			return true, nil
		}

		if strings.Contains(string(output), "No such image") {
			return false, nil
		}
	}

	return false, err
}

func gzipFile(inputFile, outputFile string) error {
	inputBytes, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	outputFileHandle, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFileHandle.Close()

	gzipWriter := gzip.NewWriter(outputFileHandle)
	defer gzipWriter.Close()

	_, err = gzipWriter.Write(inputBytes)
	if err != nil {
		return fmt.Errorf("failed to write gzipped output: %w", err)
	}

	return nil
}

func getFileSize(filename string) (int64, error) {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return 0, fmt.Errorf("failed to get file size: %w", err)
	}
	return fileInfo.Size(), nil
}
