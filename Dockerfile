# Stage 1 — Frontend build
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2 — Backend build (CGO required for go-sqlite3)
FROM golang:1.24-bookworm AS backend-builder
WORKDIR /app/backend
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev libsqlite3-dev && rm -rf /var/lib/apt/lists/*
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Copy the frontend build into the go:embed target. Must come after COPY
# backend/ or the .gitkeep placeholder would overwrite it.
COPY --from=frontend-builder /app/frontend/build ./cmd/server/ui/build/
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/server ./cmd/server

# Stage 3 — Final image (needs libc for the CGO binary)
FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=backend-builder /app/server /server
ENV CRAPCARD_DB_PATH=/data/crapcard.db
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/server"]
