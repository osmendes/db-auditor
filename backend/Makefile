.PHONY: fmt test test-cover check staticcheck lint build sqlc

VERSION ?= dev
COMMIT ?= unknown
DATE ?= unknown

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './internal/database/sqlc/*')

test:
	go test ./...

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

check:
	sqlc generate
	go vet ./...

staticcheck:
	staticcheck ./...

lint:
	golangci-lint run

build:
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X github.com/mayconmendes-qc/db-auditor/internal/buildinfo.Version=$(VERSION) -X github.com/mayconmendes-qc/db-auditor/internal/buildinfo.Commit=$(COMMIT) -X github.com/mayconmendes-qc/db-auditor/internal/buildinfo.Date=$(DATE)' -o bin/auditor ./cmd/auditor

sqlc:
	sqlc generate
