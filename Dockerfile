# Frontend build stage
FROM node:18-alpine@sha256:8d6421d663b4c28fd3ebc498332f249011d118945588d0a35cb9bc4b8ca09d9e AS build-frontend
WORKDIR /app/frontend

# Enable Corepack. The actual pnpm version comes from the `packageManager`
# field in frontend/package.json (pinned for reproducibility), so we do not
# pin a moving target like `pnpm@latest` here.
RUN corepack enable

# Copy package files first for better caching
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Copy source files and build
COPY frontend/ ./
RUN pnpm run build

# Go build stage
FROM golang:1.26.4-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS build-go
ENV CGO_ENABLED=0
ARG BUILD_VERSION

# Install git for go mod operations
RUN apk add --no-cache git

WORKDIR /app

# Set up Go module cache directory
ENV GOCACHE=/root/.cache/go-build
ENV GOMODCACHE=/root/.cache/go-mod

# Copy go.mod and go.sum first for dependency caching
COPY go.mod go.sum ./

# Download dependencies with cache mount
RUN --mount=type=cache,target=/root/.cache/go-mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download && go mod verify

# Copy source code
COPY . /app

# Copy frontend build output
COPY --from=build-frontend /app/frontend/dist /app/frontend/dist

# Build the application with cache mounts
RUN --mount=type=cache,target=/root/.cache/go-mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -mod=readonly -ldflags="-w -s -X main.version=${BUILD_VERSION}" -o hecatoncheires

# Final stage
FROM gcr.io/distroless/base:nonroot@sha256:fb282f8ed3057f71dbfe3ea0f5fa7e961415dafe4761c23948a9d4628c6166fe
USER nonroot
COPY --from=build-go /app/hecatoncheires /hecatoncheires

ENTRYPOINT ["/hecatoncheires"]
