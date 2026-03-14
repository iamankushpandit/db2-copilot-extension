# Stage 1: Build
# Use bullseye (Debian 11) for IBM DB2 CLI driver compatibility with CGO.
FROM golang:1.22-bullseye AS builder

# Install IBM DB2 CLI/ODBC driver build dependencies.
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    libxml2-dev \
    libssl-dev \
    curl \
    unzip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy Go module files and download dependencies first (layer caching).
COPY go.mod go.sum ./
RUN go mod download

# Copy source code.
COPY . .

# Build the binary with CGO enabled.
# IBM_DB_HOME must point to the clidriver installation if building locally.
# In Docker, this path is set by the go_ibm_db package's init code.
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o db2-copilot-extension .

# Stage 2: Runtime
FROM debian:bullseye-slim

# Install runtime shared libraries required by the IBM DB2 CLI driver.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libxml2 \
    libssl1.1 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user for security.
RUN useradd -r -u 1001 -g daemon appuser

WORKDIR /app

# Copy the compiled binary.
COPY --from=builder /build/db2-copilot-extension .

# Copy the IBM DB2 CLI driver libraries from the builder stage.
# The go_ibm_db package installs the clidriver under $GOPATH/pkg/mod.
# Adjust the path to match your specific version of go_ibm_db.
COPY --from=builder /root/go/pkg/mod/github.com/ibmdb/go_ibm_db*/installer/clidriver /opt/clidriver

ENV LD_LIBRARY_PATH=/opt/clidriver/lib
ENV IBM_DB_HOME=/opt/clidriver

USER appuser

EXPOSE 8080

ENTRYPOINT ["./db2-copilot-extension"]
