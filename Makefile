VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build frontend-build go-build test lint dev dev-api clean

build: frontend-build go-build

frontend-build:
	cd frontend && npm ci && npm run build

go-build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o wgpilot ./cmd/wgpilot

test:
	go test ./...

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

dev:
	cd frontend && VITE_API_PROXY=http://localhost:8080 npm run dev

dev-api:
	go run ./cmd/wgpilot serve --dev-mode --log-level=debug

clean:
	rm -rf wgpilot frontend/dist frontend/node_modules
