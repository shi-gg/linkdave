FROM golang:1.26 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libopus-dev \
    libopusfile-dev \
    && rm -rf /var/lib/apt/lists/*

RUN ARCH=$(uname -m) && \
    mkdir -p /opt/libs/lib /opt/libs/include && \
    cp /usr/lib/${ARCH}-linux-gnu/libopus.so.0*     /opt/libs/lib/ && \
    cp /usr/lib/${ARCH}-linux-gnu/libopusfile.so.0* /opt/libs/lib/ && \
    cp /usr/lib/${ARCH}-linux-gnu/libogg.so.0*      /opt/libs/lib/

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG BUILD_VERSION
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${BUILD_VERSION}" \
    -o linkdave ./cmd/linkdave

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o healthcheck ./cmd/healthcheck


FROM gcr.io/distroless/cc-debian13

COPY --from=builder /app/linkdave               /usr/local/bin/linkdave
COPY --from=builder /app/healthcheck            /usr/local/bin/healthcheck
COPY --from=builder /opt/libs/lib/libopus.so.0      /usr/lib/
COPY --from=builder /opt/libs/lib/libopusfile.so.0  /usr/lib/
COPY --from=builder /opt/libs/lib/libogg.so.0       /usr/lib/

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

USER nonroot

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/linkdave"]
