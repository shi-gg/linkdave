FROM golang:1.26 AS builder

RUN apt-get update && apt-get install -y \
    pkg-config \
    libopus-dev \
    libopusfile-dev \
    gcc \
    git \
    curl \
    unzip \
    && rm -rf /var/lib/apt/lists/*

ENV LIBDAVE_VERSION=1.1.1

RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then \
        GITHUB_ARCH="X64"; \
        LIBDAVE_SHA256="2470a131dbf39a820d893ba3d6f01373fc597868caffd6a30a8746e441ed5ede"; \
    elif [ "$ARCH" = "aarch64" ]; then \
        GITHUB_ARCH="ARM64"; \
        LIBDAVE_SHA256="2848ca62da5c8303626cfa410827f37735455f614256311fc320b35c7c6a1975"; \
    else \
        echo "Unsupported architecture: $ARCH"; exit 1; \
    fi && \
    curl -fsL "https://github.com/discord/libdave/releases/download/v${LIBDAVE_VERSION}/cpp/libdave-Linux-${GITHUB_ARCH}-boringssl.zip" -o /tmp/libdave.zip && \
    echo "${LIBDAVE_SHA256}  /tmp/libdave.zip" | sha256sum -c -

RUN mkdir -p ~/.local/lib ~/.local/include ~/.local/lib/pkgconfig && \
    unzip -j -o /tmp/libdave.zip "include/dave/dave.h" -d ~/.local/include && \
    unzip -j -o /tmp/libdave.zip "lib/libdave.so" -d ~/.local/lib && \
    rm /tmp/libdave.zip

RUN echo "prefix=$HOME/.local\nexec_prefix=\${prefix}\nlibdir=\${exec_prefix}/lib\nincludedir=\${prefix}/include\n\nName: dave\nDescription: Discord Audio & Video End-to-End Encryption (DAVE) Protocol\nVersion: ${LIBDAVE_VERSION}\nURL: https://github.com/discord/libdave\nLibs: -L\${libdir} -ldave -Wl,-rpath,\${libdir}\nCflags: -I\${includedir}" > ~/.local/lib/pkgconfig/dave.pc

ENV PKG_CONFIG_PATH="/root/.local/lib/pkgconfig"
ENV LD_LIBRARY_PATH="/root/.local/lib"

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o linkdave ./cmd/linkdave

FROM debian:trixie-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    libopus0 \
    libopusfile0 \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/linkdave /usr/local/bin/linkdave
COPY --from=builder /root/.local/lib/libdave.so /usr/local/lib/
RUN ldconfig

RUN useradd -m -s /bin/bash linkdave
USER linkdave

EXPOSE 8080

ENTRYPOINT ["linkdave"]
