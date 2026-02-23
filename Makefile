IMG ?= x402-k8s-operator:latest
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build test docker-build install-crd deploy-local undeploy sample helm-install mock-facilitator test-client lint

## Build the manager binary
build:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/manager ./cmd/manager/

## Run tests
test:
	go test ./...

## Run go vet
lint:
	go vet ./...

## Build Docker image
docker-build:
	docker build -t $(IMG) .

## Build mock-facilitator Docker image (for E2E testing)
docker-build-facilitator:
	docker build -t x402-k8s-operator-facilitator:test -f config/test/Dockerfile.mock-facilitator .

## Install CRD into the cluster
install-crd:
	kubectl apply -f config/crd/bases/

## Deploy operator locally (build image, apply manifests)
deploy-local: docker-build
	kubectl apply -f config/rbac/
	kubectl apply -f config/crd/bases/
	kubectl apply -f config/manager/

## Remove all operator resources from the cluster
undeploy:
	kubectl delete -f config/manager/ --ignore-not-found
	kubectl delete -f config/rbac/ --ignore-not-found
	kubectl delete -f config/crd/bases/ --ignore-not-found

## Apply sample X402Route
sample:
	kubectl apply -f config/samples/

## Install via Helm
helm-install:
	helm upgrade --install x402-k8s-operator ./helm/x402-k8s-operator

## Build mock-facilitator binary
mock-facilitator:
	go build -o bin/mock-facilitator ./cmd/mock-facilitator/

## Build test-client binary
test-client:
	go build -o bin/test-client ./cmd/test-client/
