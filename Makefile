APP_NAME := go-agent
CMD := ./cmd/go-agent

.PHONY: run
run:
	go run $(CMD)

.PHONY: build
build:
	mkdir -p bin
	go build -o bin/$(APP_NAME) $(CMD)

.PHONY: test
test:
	go test ./...

.PHONY: test-race
test-race:
	go test -race ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: check
check: fmt vet test

.PHONY: build-all
build-all:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/$(APP_NAME)-linux-amd64 $(CMD)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/$(APP_NAME)-darwin-arm64 $(CMD)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/$(APP_NAME)-windows-amd64.exe $(CMD)

.PHONY: clean
clean:
	rm -rf bin coverage
