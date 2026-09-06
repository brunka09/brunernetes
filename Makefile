CONTROLLER_GEN := $(shell go env GOPATH)/bin/controller-gen

.PHONY: generate manifests test fmt vet

generate:
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

manifests:
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

test:
	go test ./...

fmt:
	gofmt -w $$(find api -name '*.go' -type f)

vet:
	go vet ./...
