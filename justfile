set shell := ["bash", "-euo", "pipefail", "-c"]

default: test

# Run the sandbox from source on http://localhost:8080
run *args: build-web
    go run . {{args}}

# Build the dashboard into internal/web/dist (embedded in the binary)
build-web:
    cd web && npm ci && npm run build

# Build a single binary at bin/orchard-sandbox
build: build-web
    CGO_ENABLED=0 go build -trimpath -o bin/orchard-sandbox .

# Run the Go tests
test:
    go test -race ./...

# Run the dashboard dev server with hot reload (proxies to a sandbox on :8080)
dev-web:
    cd web && npm run dev

# Build the container image
docker tag="orchard-sandbox":
    docker build -t {{tag}} .

# Format Go code
fmt:
    gofmt -w $(git ls-files '*.go')
