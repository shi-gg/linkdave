FROM golang:1.26rc2 AS builder

# Install system dependencies
RUN apt-get update && apt-get install -y \
    pkg-config \
    libopus-dev \
    libopusfile-dev \
    gcc \
    git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o linkdave ./cmd/linkdave

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    libopus0 \
    libopusfile0 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/linkdave /usr/local/bin/linkdave

# Create non-root user
RUN useradd -m -s /bin/bash linkdave
USER linkdave

EXPOSE 8080 8081

ENTRYPOINT ["linkdave"]
