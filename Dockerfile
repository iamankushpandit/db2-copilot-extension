# ─── Build Stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-bookworm AS builder

# Install build dependencies
# libpq-dev: required for PostgreSQL client library headers
# To enable DB2 support, also install the IBM DB2 CLI driver and set IBM_DB_HOME.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Download dependencies first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application (PostgreSQL-only by default)
# To build with DB2 support: docker build --build-arg BUILD_TAGS=db2 .
ARG BUILD_TAGS=""
RUN if [ -n "$BUILD_TAGS" ]; then \
        CGO_ENABLED=1 go build -tags "$BUILD_TAGS" -ldflags="-s -w" -o db2-copilot-extension .; \
    else \
        CGO_ENABLED=0 go build -ldflags="-s -w" -o db2-copilot-extension .; \
    fi

# ─── Runtime Stage ─────────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

# Install runtime dependencies
# libpq5: PostgreSQL client library (runtime)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/db2-copilot-extension .

# Create a non-root user for security
RUN useradd -r -u 1001 -g root appuser
USER appuser

EXPOSE 8080

ENTRYPOINT ["./db2-copilot-extension"]
