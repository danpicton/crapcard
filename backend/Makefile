BUILD_FLAGS :=
export GOTOOLCHAIN := local

FRONTEND_DIR := ../frontend
UI_DEST      := cmd/server/ui/build

.PHONY: build build-prod frontend test lint run

# Development build (no frontend embed; placeholder index.html served)
build:
	CGO_ENABLED=1 go build $(BUILD_FLAGS) -o server ./cmd/server

# Production build: build frontend, copy into embed dir, then compile Go binary
build-prod: frontend
	CGO_ENABLED=1 go build $(BUILD_FLAGS) -o server ./cmd/server

# Build the SvelteKit frontend and copy output into the Go embed directory
frontend:
	cd $(FRONTEND_DIR) && npm ci && npm run build
	rm -rf $(UI_DEST)
	cp -r $(FRONTEND_DIR)/build $(UI_DEST)
	touch $(UI_DEST)/.gitkeep

test:
	CGO_ENABLED=1 go test $(BUILD_FLAGS) ./...

lint:
	golangci-lint run ./...

run:
	CGO_ENABLED=1 go run $(BUILD_FLAGS) ./cmd/server
