APP_NAME := go-agent
CMD := ./cmd/go-agent
SOAK_PACKAGES := ./internal/mmu ./internal/storage/sqlite

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

.PHONY: soak-compile
soak-compile:
	go test -tags=soak -run '^$$' $(SOAK_PACKAGES)

.PHONY: soak-smoke
soak-smoke:
	SOAK_DURATION=30s SOAK_INTERVAL=250ms SOAK_SAMPLE_INTERVAL=5s SOAK_MMU_CORPUS_PAGES=10000 go test -v -tags=soak -run '^TestSoak' -count=1 $(SOAK_PACKAGES)

.PHONY: soak-1h
soak-1h:
	SOAK_DURATION=1h SOAK_INTERVAL=250ms SOAK_SAMPLE_INTERVAL=1m SOAK_MMU_CORPUS_PAGES=100000 go test -v -tags=soak -run '^TestSoak' -count=1 -timeout=75m $(SOAK_PACKAGES)

.PHONY: soak-8h
soak-8h:
	SOAK_DURATION=8h SOAK_INTERVAL=500ms SOAK_SAMPLE_INTERVAL=5m SOAK_MMU_CORPUS_PAGES=100000 go test -v -tags=soak -run '^TestSoak' -count=1 -timeout=9h $(SOAK_PACKAGES)

.PHONY: soak-24h
soak-24h:
	SOAK_DURATION=24h SOAK_INTERVAL=1s SOAK_SAMPLE_INTERVAL=10m SOAK_MMU_CORPUS_PAGES=100000 go test -v -tags=soak -run '^TestSoak' -count=1 -timeout=25h $(SOAK_PACKAGES)

.PHONY: build-all
build-all:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/$(APP_NAME)-linux-amd64 $(CMD)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/$(APP_NAME)-darwin-arm64 $(CMD)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/$(APP_NAME)-windows-amd64.exe $(CMD)

.PHONY: clean
clean:
	rm -rf bin coverage
