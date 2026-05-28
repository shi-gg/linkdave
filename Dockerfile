FROM golang:1.26 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libopus-dev \
    libopusfile-dev \
    gcc \
    curl \
    unzip \
    && rm -rf /var/lib/apt/lists/*

RUN ARCH=$(uname -m) && \
    mkdir -p /opt/libs/lib /opt/libs/include && \
    cp /usr/lib/${ARCH}-linux-gnu/libopus.so.0*     /opt/libs/lib/ && \
    cp /usr/lib/${ARCH}-linux-gnu/libopusfile.so.0* /opt/libs/lib/ && \
    cp /usr/lib/${ARCH}-linux-gnu/libogg.so.0*      /opt/libs/lib/

ENV LIBDAVE_VERSION=1.1.1

RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then \
        GITHUB_ARCH="X64"; \
        SHA256="2470a131dbf39a820d893ba3d6f01373fc597868caffd6a30a8746e441ed5ede"; \
    elif [ "$ARCH" = "aarch64" ]; then \
        GITHUB_ARCH="ARM64"; \
        SHA256="2848ca62da5c8303626cfa410827f37735455f614256311fc320b35c7c6a1975"; \
    else \
        echo "Unsupported architecture: $ARCH" && exit 1; \
    fi && \
    curl -fsL "https://github.com/discord/libdave/releases/download/v${LIBDAVE_VERSION}/cpp/libdave-Linux-${GITHUB_ARCH}-boringssl.zip" \
        -o /tmp/libdave.zip && \
    echo "${SHA256}  /tmp/libdave.zip" | sha256sum -c - && \
    unzip -j /tmp/libdave.zip "include/dave/dave.h" -d /opt/libs/include && \
    unzip -j /tmp/libdave.zip "lib/libdave.so"      -d /opt/libs/lib && \
    rm /tmp/libdave.zip

RUN mkdir -p /opt/libs/lib/pkgconfig && printf "\
prefix=/opt/libs\n\
exec_prefix=\${prefix}\n\
libdir=\${exec_prefix}/lib\n\
includedir=\${prefix}/include\n\
\n\
Name: dave\n\
Description: Discord DAVE Protocol\n\
Version: %s\n\
Libs: -L\${libdir} -ldave -Wl,-rpath,/usr/lib\n\
Cflags: -I\${includedir}\n" "${LIBDAVE_VERSION}" > /opt/libs/lib/pkgconfig/dave.pc

ENV PKG_CONFIG_PATH=/opt/libs/lib/pkgconfig
ENV LD_LIBRARY_PATH=/opt/libs/lib

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
COPY --from=builder /opt/libs/lib/libdave.so        /usr/lib/
COPY --from=builder /opt/libs/lib/libopus.so.0      /usr/lib/
COPY --from=builder /opt/libs/lib/libopusfile.so.0  /usr/lib/
COPY --from=builder /opt/libs/lib/libogg.so.0       /usr/lib/

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

USER nonroot

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/linkdave"]
