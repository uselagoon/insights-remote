SHELL := /bin/bash

# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

INGRESS_VERSION=4.9.1

KIND_CLUSTER ?= insights-remote
KIND_NETWORK ?= insights-remote

TIMEOUT = 30m

KIND_VERSION = v0.27.0
KUBECTL_VERSION := v1.32.3
HELM_VERSION := v3.16.1
GOJQ_VERSION = v0.12.16

HELM = $(realpath ./local-dev/helm)
KUBECTL = $(realpath ./local-dev/kubectl)
JQ = $(realpath ./local-dev/jq)
KIND = $(realpath ./local-dev/kind)

ARCH := $(shell uname | tr '[:upper:]' '[:lower:]')

.PHONY: local-dev/kind
local-dev/kind:
ifeq ($(KIND_VERSION), $(shell kind version 2>/dev/null | sed -nE 's/kind (v[0-9.]+).*/\1/p'))
	$(info linking local kind version $(KIND_VERSION))
	$(eval KIND = $(realpath $(shell command -v kind)))
else
ifneq ($(KIND_VERSION), $(shell ./local-dev/kind version 2>/dev/null | sed -nE 's/kind (v[0-9.]+).*/\1/p'))
	$(info downloading kind version $(KIND_VERSION) for $(ARCH))
	mkdir -p local-dev
	rm local-dev/kind || true
	curl -sSLo local-dev/kind https://kind.sigs.k8s.io/dl/$(KIND_VERSION)/kind-$(ARCH)-amd64
	chmod a+x local-dev/kind
endif
endif

.PHONY: local-dev/helm
local-dev/helm:
ifeq ($(HELM_VERSION), $(shell helm version --short --client 2>/dev/null | sed -nE 's/(v[0-9.]+).*/\1/p'))
	$(info linking local helm version $(HELM_VERSION))
	$(eval HELM = $(realpath $(shell command -v helm)))
else
ifneq ($(HELM_VERSION), $(shell ./local-dev/helm version --short --client 2>/dev/null | sed -nE 's/(v[0-9.]+).*/\1/p'))
	$(info downloading helm version $(HELM_VERSION) for $(ARCH))
	rm local-dev/helm || true
	curl -sSL https://get.helm.sh/helm-$(HELM_VERSION)-$(ARCH)-amd64.tar.gz | tar -xzC local-dev --strip-components=1 $(ARCH)-amd64/helm
	chmod a+x local-dev/helm
endif
endif

.PHONY: local-dev/jq
local-dev/jq:
ifeq ($(GOJQ_VERSION), $(shell gojq -v 2>/dev/null | sed -nE 's/gojq ([0-9.]+).*/v\1/p'))
	$(info linking local gojq version $(GOJQ_VERSION))
	$(eval JQ = $(realpath $(shell command -v gojq)))
else
ifneq ($(GOJQ_VERSION), $(shell ./local-dev/jq -v 2>/dev/null | sed -nE 's/gojq ([0-9.]+).*/v\1/p'))
	$(info downloading gojq version $(GOJQ_VERSION) for $(ARCH))
	mkdir -p local-dev
	rm local-dev/jq || true
ifeq ($(ARCH), darwin)
	TMPDIR=$$(mktemp -d) \
		&& curl -sSL https://github.com/itchyny/gojq/releases/download/$(GOJQ_VERSION)/gojq_$(GOJQ_VERSION)_$(ARCH)_arm64.zip -o $$TMPDIR/gojq.zip \
		&& (cd $$TMPDIR && unzip gojq.zip) && cp $$TMPDIR/gojq_$(GOJQ_VERSION)_$(ARCH)_arm64/gojq ./local-dev/jq && rm -rf $$TMPDIR
else
	curl -sSL https://github.com/itchyny/gojq/releases/download/$(GOJQ_VERSION)/gojq_$(GOJQ_VERSION)_$(ARCH)_amd64.tar.gz | tar -xzC local-dev --strip-components=1 gojq_$(GOJQ_VERSION)_$(ARCH)_amd64/gojq
	mv ./local-dev/{go,}jq
endif
	chmod a+x local-dev/jq
endif
endif

.PHONY: local-dev/kubectl
local-dev/kubectl:
ifeq ($(KUBECTL_VERSION), $(shell kubectl version --client 2>/dev/null | grep Client | sed -E 's/Client Version: (v[0-9.]+).*/\1/'))
	$(info linking local kubectl version $(KUBECTL_VERSION))
	$(eval KUBECTL = $(realpath $(shell command -v kubectl)))
else
ifneq ($(KUBECTL_VERSION), $(shell ./local-dev/kubectl version --client 2>/dev/null | grep Client | sed -E 's/Client Version: (v[0-9.]+).*/\1/'))
	$(info downloading kubectl version $(KUBECTL_VERSION) for $(ARCH))
	mkdir -p local-dev
	rm local-dev/kubectl || true
	curl -sSLo local-dev/kubectl https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(ARCH)/amd64/kubectl
	chmod a+x local-dev/kubectl
endif
endif

.PHONY: local-dev/tools
local-dev/tools: local-dev/kind local-dev/kubectl local-dev/jq local-dev/helm 

.PHONY: helm/repos
helm/repos: local-dev/helm
	# install repo dependencies required by the charts
	$(HELM) repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
	$(HELM) repo add metallb https://metallb.github.io/metallb
	$(HELM) repo update

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name insights-remote-builder
	$(CONTAINER_TOOL) buildx use insights-remote-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm insights-remote-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.5.0
CONTROLLER_TOOLS_VERSION ?= v0.17.2
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v1.63.4

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: create-kind-cluster
create-kind-cluster: local-dev/tools helm/repos
	docker network inspect $(KIND_NETWORK) >/dev/null || docker network create $(KIND_NETWORK) \
		&& export KIND_EXPERIMENTAL_DOCKER_NETWORK=$(KIND_NETWORK) \
 		&& $(KIND) create cluster --wait=60s --name=$(KIND_CLUSTER) --config=test-resources/test-suite.kind-config.yaml
# 	LAGOON_KIND_CIDR_BLOCK=$$(docker network inspect $(KIND_NETWORK) | $(JQ) '.[].Containers[].IPv4Address' | tr -d '"') \
# 		&& export KIND_NODE_IP=$$(echo $${LAGOON_KIND_CIDR_BLOCK%???} | awk -F'.' '{print $$1,$$2,$$3,240}' OFS='.') \
#  		&& envsubst < test/e2e/testdata/example-nginx.yaml.tpl > test/e2e/testdata/example-nginx.yaml

# generate-broker-certs will generate a ca, server and client certificate used for the test suite
.PHONY: generate-broker-certs
generate-broker-certs:
	@mkdir -p local-dev/certificates
	openssl x509 -enddate -noout -in local-dev/certificates/ca.crt > /dev/null 2>&1 || \
    (openssl genrsa -out local-dev/certificates/ca.key 4096 && \
	openssl req -x509 -new -nodes -key local-dev/certificates/ca.key -sha256 -days 1826 -out local-dev/certificates/ca.crt -subj '/CN=lagoon Root CA/C=IO/ST=State/L=City/O=lagoon' && \
    openssl ecparam -name prime256v1 -genkey -noout -out local-dev/certificates/tls.key && \
	chmod +r local-dev/certificates/tls.key && \
	echo "subjectAltName = IP:172.17.0.1" > local-dev/certificates/extfile.cnf && \
	openssl req -new -nodes -out local-dev/certificates/tls.csr -key local-dev/certificates/tls.key -subj '/CN=broker/C=IO/ST=State/L=City/O=lagoon' && \
    openssl x509 -req -in local-dev/certificates/tls.csr -CA local-dev/certificates/ca.crt -extfile local-dev/certificates/extfile.cnf -CAkey local-dev/certificates/ca.key -CAcreateserial -out local-dev/certificates/tls.crt -days 730 -sha256 && \
    openssl ecparam -name prime256v1 -genkey -noout -out local-dev/certificates/clienttls.key && \
	chmod +r local-dev/certificates/clienttls.key && \
	openssl req -new -nodes -out local-dev/certificates/clienttls.csr -key local-dev/certificates/clienttls.key -subj '/CN=client/C=IO/ST=State/L=City/O=lagoon' && \
    openssl x509 -req -in local-dev/certificates/clienttls.csr -CA local-dev/certificates/ca.crt -CAkey local-dev/certificates/ca.key -CAcreateserial -out local-dev/certificates/clienttls.crt -days 730 -sha256)

.PHONY: regenerate-broker-certs
regenerate-broker-certs:
	@mkdir -p local-dev/certificates
	@rm local-dev/certificates/ca.key || true && \
	rm local-dev/certificates/ca.crt || true && \
	$(MAKE) generate-broker-certs

# Create a kind cluster locally and run the test e2e test suite against it
.PHONY: kind/test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up locally
kind/test-e2e: create-kind-cluster install-ingress kind/re-test-e2e

.PHONY: local-kind/test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up locally
kind/re-test-e2e:
	export KIND_PATH=$(KIND) && \
	export KUBECTL_PATH=$(KUBECTL) && \
	export KIND_CLUSTER=$(KIND_CLUSTER) && \
	LAGOON_KIND_CIDR_BLOCK=$$(docker network inspect $(KIND_NETWORK) | $(JQ) '.[].Containers[].IPv4Address' | tr -d '"') && \
	export KIND_NODE_IP=$$(echo $${LAGOON_KIND_CIDR_BLOCK%???} | awk -F'.' '{print $$1,$$2,$$3,240}' OFS='.') && \
	$(KIND) export kubeconfig --name=$(KIND_CLUSTER) && \
	$(MAKE) test-e2e

.PHONY: clean
kind/clean:
	docker compose down && \
		$(KIND) delete cluster --name=$(KIND_CLUSTER) && docker network rm $(KIND_NETWORK)

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
.PHONY: test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up inside github action.
test-e2e: generate-broker-certs
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: github/test-e2e
github/test-e2e: local-dev/tools install-ingress test-e2e

.PHONY: kind/set-kubeconfig
kind/set-kubeconfig:
	export KIND_CLUSTER=$(KIND_CLUSTER) && \
	$(KIND) export kubeconfig --name=$(KIND_CLUSTER)

.PHONY: kind/logs-insights-remote
kind/logs-insights-remote:
	export KIND_CLUSTER=$(KIND_CLUSTER) && \
	$(KIND) export kubeconfig --name=$(KIND_CLUSTER) && \
	$(KUBECTL) -n insights-remote-system logs -f \
		$$($(KUBECTL) -n insights-remote-system  get pod -l control-plane=controller-manager -o jsonpath="{.items[0].metadata.name}") \
		-c manager

.PHONY: install-metallb
install-metallb:
	LAGOON_KIND_CIDR_BLOCK=$$(docker network inspect $(KIND_NETWORK) | $(JQ) '.[].Containers[].IPv4Address' | tr -d '"') && \
	export LAGOON_KIND_NETWORK_RANGE=$$(echo $${LAGOON_KIND_CIDR_BLOCK%???} | awk -F'.' '{print $$1,$$2,$$3,240}' OFS='.')/29 && \
	$(HELM) upgrade \
		--install \
		--create-namespace \
		--namespace metallb-system  \
		--wait \
		--timeout $(TIMEOUT) \
		--version=v0.13.12 \
		metallb \
		metallb/metallb && \
	$$(envsubst < test-resources/test-suite.metallb-pool.yaml.tpl > test-resources/test-suite.metallb-pool.yaml) && \
	$(KUBECTL) apply -f test-resources/test-suite.metallb-pool.yaml

# installs ingress-nginx
.PHONY: install-ingress
install-ingress: install-metallb
	$(HELM) upgrade \
		--install \
		--create-namespace \
		--namespace ingress-nginx \
		--wait \
		--timeout $(TIMEOUT) \
		--set controller.allowSnippetAnnotations=true \
		--set controller.service.type=LoadBalancer \
		--set controller.service.nodePorts.http=32080 \
		--set controller.service.nodePorts.https=32443 \
		--set controller.config.proxy-body-size=0 \
		--set controller.config.hsts="false" \
		--set controller.watchIngressWithoutClass=true \
		--set controller.ingressClassResource.default=true \
		--version=$(INGRESS_VERSION) \
		ingress-nginx \
		ingress-nginx/ingress-nginx