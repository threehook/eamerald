SHELL              := $(shell which bash)

NO_COLOR           := \033[0m
OK_COLOR           := \033[32;01m
ERR_COLOR          := \033[31;01m
WARN_COLOR         := \033[36;01m
ATTN_COLOR         := \033[33;01m

REGISTRY           := ghcr.io
ORG                := threehook
REPO               := eamerald
IMAGE_ORG          := threehook
IMAGE_REPO         := eamerald
DESCRIPTION        := "Eamerald Authorization Service"
LICENSE            := Apache-2.0

GOOS               := $(shell go env GOOS)
GOARCH             := $(shell go env GOARCH)
EAMERALD_DIST      := ${PWD}/$(shell cat dist/artifacts.json | jq -r '.[] | select(.name == "mrld").path')
DOCKER_BUILDKIT    := 1

EXT_DIR            := ${PWD}/.ext
EXT_BIN_DIR        := ${EXT_DIR}/bin
EXT_TMP_DIR        := ${EXT_DIR}/tmp

GO_VER             := 1.27
SVU_VER            := 3.4.1
GOTESTSUM_VER      := 1.13.0
GOLANGCI-LINT_VER  := 2.13.2
GORELEASER_VER     := 2.18.0
SYFT_VER           := 1.13.0

RELEASE_TAG        := $$(${EXT_BIN_DIR}/svu current)

K8S_NAMESPACE          := eamerald
K8S_HUB_RELEASE        := eamerald-hub
K8S_EDGE_RELEASE       := eamerald-edge
K8S_STANDALONE_RELEASE := eamerald-standalone
K8S_HUB_CHART          := k8s/eamerald-hub
K8S_EDGE_CHART         := k8s/eamerald-edge
K8S_STANDALONE_CHART   := k8s/eamerald-standalone
K8S_DEV_IMAGE          := eamerald:dev

# MANIFEST=<path>: the directory model to deploy.
# DATA="<path> <path>": directory data files to import once deployed.
MANIFEST           ?=
DATA               ?=

OBS_NAMESPACE      := observability
OBS_RELEASE        := observability
OBS_CHART          := k8s/observability

.DEFAULT_GOAL      := build

export TESTCONTAINERS_RYUK_DISABLED=$(shell docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null | grep -q ".colima" && echo "true" || echo "false")

.PHONY: deps
deps: info install-svu install-goreleaser install-golangci-lint install-gotestsum install-syft
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"

.PHONY: gover
gover:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@(go env GOVERSION | grep "go${GO_VER}") || (echo "go version check failed expected go${GO_VER} got $$(go env GOVERSION)"; exit 1)

.PHONY: build
build: gover
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@${EXT_BIN_DIR}/goreleaser build --config .goreleaser.yml --clean --snapshot --single-target

.PHONY: docker-build
docker-build:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@docker buildx build \
  --platform=linux/arm64,linux/amd64 \
  --tag ${REGISTRY}/${ORG}/${REPO}:$$(svu current) \
	--tag ${REGISTRY}/${ORG}/${REPO}:latest  \
	--progress=plain \
  --build-arg BUILD_DATE=$$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
	--build-arg TITLE=${REPO} \
  --build-arg VCS_REF=$$(git rev-parse HEAD) \
  --build-arg VERSION=$$(svu current) \
  --build-arg REPO_URL="https://github.com/${ORG}/${REPO}" \
  --build-arg DESCRIPTION=${DESCRIPTION} \
  --build-arg LICENSE=${LICENSE} \
  --push .

.PHONY: docker-build-test
docker-build-test:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@docker buildx build \
  --platform=linux/${GOARCH} \
  --tag ${REGISTRY}/${IMAGE_ORG}/${IMAGE_REPO}:0.0.0-test-$$(git rev-parse --short HEAD)-$(GOARCH) \
	--progress=plain \
  --build-arg BUILD_DATE=$$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
	--build-arg TITLE=${IMAGE_REPO} \
  --build-arg VCS_REF=$$(git rev-parse HEAD) \
  --build-arg VERSION=$$(svu current) \
  --build-arg REPO_URL="https://github.com/${IMAGE_ORG}/${IMAGE_REPO}" \
  --build-arg DESCRIPTION=${DESCRIPTION} \
  --build-arg LICENSE=${LICENSE} \
  .

.PHONY: k8s-build
k8s-build:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@docker build -f k8s/Dockerfile.dev -t ${K8S_DEV_IMAGE} .

# vendors eamerald-common into every chart that depends on it. Without this, an install/upgrade uses whatever was
# last vendored under charts/*.tgz, which silently drifts from k8s/eamerald-common's current templates.
.PHONY: k8s-chart-deps
k8s-chart-deps:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@helm dependency update ${K8S_HUB_CHART}
	@helm dependency update ${K8S_EDGE_CHART}
	@helm dependency update ${K8S_STANDALONE_CHART}

.PHONY: k8s-install
k8s-install: require-manifest k8s-chart-deps
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@helm upgrade --install ${K8S_HUB_RELEASE} ${K8S_HUB_CHART} -n ${K8S_NAMESPACE} --create-namespace \
		--set-file directory.manifest.content=$(MANIFEST)
	@kubectl -n ${K8S_NAMESPACE} rollout status deployment/${K8S_HUB_RELEASE}
	@helm upgrade --install ${K8S_EDGE_RELEASE} ${K8S_EDGE_CHART} -n ${K8S_NAMESPACE} --create-namespace \
		--set edge.hub.address=${K8S_HUB_RELEASE}.${K8S_NAMESPACE}.svc.cluster.local:9292

.PHONY: k8s-install-standalone
k8s-install-standalone: require-manifest k8s-chart-deps
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@helm upgrade --install ${K8S_STANDALONE_RELEASE} ${K8S_STANDALONE_CHART} -n ${K8S_NAMESPACE} --create-namespace \
		--set-file directory.manifest.content=$(MANIFEST)

# the chart cannot render without a directory model: an init container applies it before eameraldd starts, and a directory without one rejects
# every write.
.PHONY: require-manifest
require-manifest:
	@if [ -z "$(MANIFEST)" ]; then \
		echo -e "$(ERR_COLOR)MANIFEST is required, e.g. MANIFEST=examples/laadpalen/manifest.yaml$(NO_COLOR)"; \
		exit 1; \
	fi

# k8s-deploy builds under a fresh, unique tag every run - Docker Desktop caches images by tag, so a static tag can silently never reach the Pod.
# Deploys the hub (seeded with MANIFEST/DATA), then an edge synced from it - the default two-chart local topology (see
# docs/deployments/k8s-hub-edge.md). DATA is imported into the hub before the edge is (re)deployed, so the edge's first sync already
# picks it up; waiting on its rollout afterwards also waits out that sync, since its readinessProbe is pinned to the "sync" health check.
# --reset-then-reuse-values (not --reuse-values): keeps new values.yaml keys defaulted instead of empty on an existing release.
.PHONY: k8s-deploy
k8s-deploy: require-manifest k8s-chart-deps
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@echo "deploying with MANIFEST=$(MANIFEST) - wiping the hub's existing directory data"
	@kubectl -n ${K8S_NAMESPACE} scale deployment/${K8S_HUB_RELEASE} --replicas=0 2>/dev/null || true
	@kubectl -n ${K8S_NAMESPACE} wait --for=delete pod -l app.kubernetes.io/instance=${K8S_HUB_RELEASE} --timeout=60s 2>/dev/null || true
	@kubectl -n ${K8S_NAMESPACE} delete pvc ${K8S_HUB_RELEASE}-db --ignore-not-found
	@TAG=dev-$$(git rev-parse --short HEAD)-$$(date +%s); \
	echo "building eamerald:$$TAG"; \
	docker build -f k8s/Dockerfile.dev -t eamerald:$$TAG . && \
	helm upgrade --install ${K8S_HUB_RELEASE} ${K8S_HUB_CHART} -n ${K8S_NAMESPACE} --create-namespace --reset-then-reuse-values \
		--set image.tag=$$TAG \
		--set-file directory.manifest.content=$(MANIFEST) && \
	kubectl -n ${K8S_NAMESPACE} rollout restart deployment/${K8S_HUB_RELEASE} && \
	kubectl -n ${K8S_NAMESPACE} rollout status deployment/${K8S_HUB_RELEASE} && \
	if [ -n "$(DATA)" ]; then \
		echo "importing data: $(DATA)"; \
		cat $(DATA) | go run ./mrld directory import --stdin -H localhost:9292 --insecure; \
	fi; \
	helm upgrade --install ${K8S_EDGE_RELEASE} ${K8S_EDGE_CHART} -n ${K8S_NAMESPACE} --create-namespace --reset-then-reuse-values \
		--set image.tag=$$TAG \
		--set edge.hub.address=${K8S_HUB_RELEASE}.${K8S_NAMESPACE}.svc.cluster.local:9292 && \
	kubectl -n ${K8S_NAMESPACE} rollout restart deployment/${K8S_EDGE_RELEASE}
	@kubectl -n ${K8S_NAMESPACE} rollout status deployment/${K8S_EDGE_RELEASE}

# the one-pod all-in-one flow, for a quick local check that doesn't need the hub/edge split.
.PHONY: k8s-deploy-standalone
k8s-deploy-standalone: require-manifest k8s-chart-deps
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@echo "deploying with MANIFEST=$(MANIFEST) - wiping the existing directory data"
	@kubectl -n ${K8S_NAMESPACE} scale deployment/${K8S_STANDALONE_RELEASE} --replicas=0 2>/dev/null || true
	@kubectl -n ${K8S_NAMESPACE} wait --for=delete pod -l app.kubernetes.io/instance=${K8S_STANDALONE_RELEASE} --timeout=60s 2>/dev/null || true
	@kubectl -n ${K8S_NAMESPACE} delete pvc ${K8S_STANDALONE_RELEASE}-db --ignore-not-found
	@TAG=dev-$$(git rev-parse --short HEAD)-$$(date +%s); \
	echo "building eamerald:$$TAG"; \
	docker build -f k8s/Dockerfile.dev -t eamerald:$$TAG . && \
	helm upgrade --install ${K8S_STANDALONE_RELEASE} ${K8S_STANDALONE_CHART} -n ${K8S_NAMESPACE} --create-namespace --reset-then-reuse-values \
		--set image.tag=$$TAG \
		--set-file directory.manifest.content=$(MANIFEST) && \
	kubectl -n ${K8S_NAMESPACE} rollout restart deployment/${K8S_STANDALONE_RELEASE}
	@kubectl -n ${K8S_NAMESPACE} rollout status deployment/${K8S_STANDALONE_RELEASE}
	@if [ -n "$(DATA)" ]; then \
		echo "importing data: $(DATA)"; \
		cat $(DATA) | go run ./mrld directory import --stdin -H localhost:9292 --insecure; \
	fi

.PHONY: k8s-uninstall
k8s-uninstall:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@helm uninstall ${K8S_EDGE_RELEASE} -n ${K8S_NAMESPACE} --ignore-not-found
	@helm uninstall ${K8S_HUB_RELEASE} -n ${K8S_NAMESPACE} --ignore-not-found

.PHONY: k8s-uninstall-standalone
k8s-uninstall-standalone:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@helm uninstall ${K8S_STANDALONE_RELEASE} -n ${K8S_NAMESPACE}

# installs Loki, Alloy and Grafana, and points the running edge's ADL logger at Alloy so decision records start being exported over OTLP.
# The hub has no authorizer and never logs decisions, so it's untouched here.
.PHONY: k8s-observability-install
k8s-observability-install:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@helm upgrade --install ${OBS_RELEASE} ${OBS_CHART} -n ${OBS_NAMESPACE} --create-namespace --wait
	@helm upgrade --install ${K8S_EDGE_RELEASE} ${K8S_EDGE_CHART} -n ${K8S_NAMESPACE} --create-namespace --reset-then-reuse-values \
		--set adlDecisionLogger.otlp.endpoint=alloy.${OBS_NAMESPACE}.svc.cluster.local:4317
	@kubectl -n ${K8S_NAMESPACE} rollout status deployment/${K8S_EDGE_RELEASE}

.PHONY: k8s-observability-uninstall
k8s-observability-uninstall:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@helm uninstall ${OBS_RELEASE} -n ${OBS_NAMESPACE}

# opens Grafana on http://localhost:3000; ADL records are in Explore, under
# the Loki datasource, as {service_name="eamerald"}.
.PHONY: k8s-grafana
k8s-grafana:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@kubectl -n ${OBS_NAMESPACE} port-forward svc/grafana 3000:3000

.PHONY: k8s-status
k8s-status:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@kubectl -n ${K8S_NAMESPACE} get pods,svc

.PHONY: k8s-logs-hub
k8s-logs-hub:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@kubectl -n ${K8S_NAMESPACE} logs -f deployment/${K8S_HUB_RELEASE}

.PHONY: k8s-logs-edge
k8s-logs-edge:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@kubectl -n ${K8S_NAMESPACE} logs -f deployment/${K8S_EDGE_RELEASE}

.PHONY: k8s-logs-standalone
k8s-logs-standalone:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@kubectl -n ${K8S_NAMESPACE} logs -f deployment/${K8S_STANDALONE_RELEASE}

# laadpalen deploys via k8s-deploy like every other manifest - see examples/laadpalen/README.md:
#   make k8s-deploy MANIFEST=examples/laadpalen/manifest.yaml \
#     DATA="examples/laadpalen/laadpalen_objects.jsonl examples/laadpalen/laadpalen_relations.jsonl"
.PHONY: laadpalen-gui
laadpalen-gui:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@cd examples/laadpalen/gui && npm install && npm run dev

# checks every case in examples/laadpalen/test_cases.json against a running authorizer's request_laadpaal decision
# (see examples/laadpalen/README.md for how to deploy with that model first).
.PHONY: laadpalen-test
laadpalen-test:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@examples/laadpalen/test.sh

PHONY: go-mod-tidy
go-mod-tidy:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@go work edit -json | jq -r '.Use[].DiskPath' | xargs -I{} bash -c 'cd {} && echo "${PWD}/go.mod" && go mod tidy -v && cd -'

.PHONY: release
release: gover
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@${EXT_BIN_DIR}/goreleaser release --config .goreleaser.yml --clean

.PHONY: snapshot
snapshot: gover
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@${EXT_BIN_DIR}/goreleaser release --config .goreleaser.yml --clean --snapshot --skip sbom

.PHONY: generate
generate:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@GOBIN=${EXT_BIN_DIR} go generate ./...

.PHONY: lint
lint: gover
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@${EXT_BIN_DIR}/golangci-lint config path
	@${EXT_BIN_DIR}/golangci-lint config verify
	@${EXT_BIN_DIR}/golangci-lint run --config ${PWD}/.golangci.yaml

.PHONY: lint-clean
lint-clean: gover
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@${EXT_BIN_DIR}/golangci-lint cache clean

.PHONY: test
test: gover test-snapshot
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@echo -e "$(WARN_COLOR)!!! TESTCONTAINERS_RYUK_DISABLED=${TESTCONTAINERS_RYUK_DISABLED} !!!$(NO_COLOR)"
	@${EXT_BIN_DIR}/gotestsum --format short-verbose -- $$(go list ./... | grep -v daemon/tests)                     -count=1 -timeout 120s --race -parallel=1 -v -coverprofile=cover.out -coverpkg=./...
	@${EXT_BIN_DIR}/gotestsum --format short-verbose -- $$(go list ./daemon/tests/... | grep -v tests/template)      -count=1 -timeout 120s --race -parallel=1 -v -coverprofile=cover.out -coverpkg=./...
	@${EXT_BIN_DIR}/gotestsum --format short-verbose -- github.com/${ORG}/${REPO}/daemon/tests/template-no-tls/...   -count=1 -timeout 120s --race -parallel=1 -v -coverprofile=cover.out -coverpkg=./...
	@${EXT_BIN_DIR}/gotestsum --format short-verbose -- github.com/${ORG}/${REPO}/daemon/tests/template-with-tls/... -count=1 -timeout 120s --race -parallel=1 -v -coverprofile=cover.out -coverpkg=./...
	
.PHONY: test-snapshot
test-snapshot:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@docker image rm ${REGISTRY}/${IMAGE_ORG}/${IMAGE_REPO}:0.0.0-test-$$(git rev-parse --short HEAD)-$$(uname -m) || true
	@${EXT_BIN_DIR}/goreleaser release --config .goreleaser-test.yml --clean --snapshot --skip archive,sbom

.PHONE: container-tag
container-tag:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@scripts/container-tag.sh > .container-tag.env

.PHONY: write-version
write-version:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@git describe --tags > ./VERSION.txt

.PHONY: eamerald-run-test-snapshot
eamerald-run-test-snapshot:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@echo "mrld run $$(${EAMERALD_DIST} config info | jq '.runtime.active_configuration_file')"
	@${EAMERALD_DIST} run --container-tag=0.0.0-test-$$(git rev-parse --short HEAD)-$$(uname -m)

.PHONY: eamerald-start-test-snapshot
eamerald-start-test-snapshot:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@echo "mrld start $$(${EAMERALD_DIST} config info | jq '.runtime.active_configuration_name')"
	@${EAMERALD_DIST} start --container-tag=0.0.0-test-$$(git rev-parse --short HEAD)-$$(uname -m)

.PHONY: info
info:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@echo "GOOS:        ${GOOS}"
	@echo "GOARCH:      ${GOARCH}"
	@echo "EXT_DIR:     ${EXT_DIR}"
	@echo "EXT_BIN_DIR: ${EXT_BIN_DIR}"
	@echo "EXT_TMP_DIR: ${EXT_TMP_DIR}"
	@echo "RELEASE_TAG: ${RELEASE_TAG}"
	@echo "EAMERALD_DIST: ${EAMERALD_DIST}"
	@echo "REGISTRY:    ${REGISTRY}"
	@echo "ORG:         ${ORG}"
	@echo "REPO:        ${REPO}"
	@echo "COLIMA:      ${IS_COLIMA}"

.PHONY: install-svu
install-svu: ${EXT_BIN_DIR} ${EXT_TMP_DIR}
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@GOBIN=${EXT_BIN_DIR} go install github.com/caarlos0/svu/v3@v${SVU_VER}
	@${EXT_BIN_DIR}/svu --version

.PHONY: install-gotestsum
install-gotestsum: ${EXT_TMP_DIR} ${EXT_BIN_DIR}
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@gh release download v${GOTESTSUM_VER} --repo https://github.com/gotestyourself/gotestsum --pattern "gotestsum_${GOTESTSUM_VER}_${GOOS}_${GOARCH}.tar.gz" --output "${EXT_TMP_DIR}/gotestsum.tar.gz" --clobber
	@tar -xvf ${EXT_TMP_DIR}/gotestsum.tar.gz --directory ${EXT_BIN_DIR} gotestsum &> /dev/null
	@chmod +x ${EXT_BIN_DIR}/gotestsum
	@${EXT_BIN_DIR}/gotestsum --version

.PHONY: install-golangci-lint
install-golangci-lint: ${EXT_TMP_DIR} ${EXT_BIN_DIR}
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@gh release download v${GOLANGCI-LINT_VER} --repo https://github.com/golangci/golangci-lint --pattern "golangci-lint-${GOLANGCI-LINT_VER}-${GOOS}-${GOARCH}.tar.gz" --output "${EXT_TMP_DIR}/golangci-lint.tar.gz" --clobber
	@tar --strip=1 -xvf ${EXT_TMP_DIR}/golangci-lint.tar.gz --strip-components=1 --directory ${EXT_TMP_DIR} &> /dev/null
	@mv ${EXT_TMP_DIR}/golangci-lint ${EXT_BIN_DIR}/golangci-lint
	@chmod +x ${EXT_BIN_DIR}/golangci-lint
	@${EXT_BIN_DIR}/golangci-lint --version

.PHONY: install-goreleaser
install-goreleaser: ${EXT_TMP_DIR} ${EXT_BIN_DIR}
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@gh release download v${GORELEASER_VER} --repo https://github.com/goreleaser/goreleaser --pattern "goreleaser_$$(uname -s)_$$(uname -m).tar.gz" --output "${EXT_TMP_DIR}/goreleaser.tar.gz" --clobber
	@tar -xvf ${EXT_TMP_DIR}/goreleaser.tar.gz --directory ${EXT_BIN_DIR} goreleaser &> /dev/null
	@chmod +x ${EXT_BIN_DIR}/goreleaser
	@${EXT_BIN_DIR}/goreleaser --version

.PHONY: install-syft
install-syft: ${EXT_TMP_DIR} ${EXT_BIN_DIR}
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@gh release download v${SYFT_VER} --repo https://github.com/anchore/syft --pattern "syft_${SYFT_VER}_${GOOS}_${GOARCH}.tar.gz" --output "${EXT_TMP_DIR}/syft.tar.gz" --clobber
	@tar -xvf ${EXT_TMP_DIR}/syft.tar.gz --directory ${EXT_BIN_DIR} syft &> /dev/null
	@chmod +x ${EXT_BIN_DIR}/syft
	@${EXT_BIN_DIR}/syft --version

.PHONY: clean
clean:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@rm -rf ${EXT_DIR}
	@rm -rf ${BIN_DIR}
	@rm -rf ./dist
	@rm -rf ./test

${BIN_DIR}:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@mkdir -p ${BIN_DIR}

${EXT_BIN_DIR}:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@mkdir -p ${EXT_BIN_DIR}

${EXT_TMP_DIR}:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@mkdir -p ${EXT_TMP_DIR}
