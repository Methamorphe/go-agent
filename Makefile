APP_NAME := go-agent
CMD := ./cmd/go-agent

.PHONY: run
run:
	go run $(CMD)

.PHONY: build
build:
	go build -o $(APP_NAME) $(CMD)

.PHONY: test
test:
	go test ./...

.PHONY: test-race
test-race:
	go test -race ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: check
check: fmt vet test
