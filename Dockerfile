FROM rust:1.95-slim AS builder

# Install system build dependencies for ORT, OpenSSL, and Tokenizers
RUN apt-get update && apt-get install -y \
    pkg-config \
    libssl-dev \
    clang \
    cmake \
    g++ \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Take help from .dockerignore before COPY
COPY . .

WORKDIR /app/server
RUN ls -lhA /app

# Caching Rust cargo
ENV CARGO_TARGET_DIR=/tmp/cargo-target

RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/usr/local/cargo/git \
    --mount=type=cache,target=/tmp/cargo-target \
    cargo build --release && \
    cp /tmp/cargo-target/release/asr-server /usr/local/bin/asr-server

FROM scratch AS export-stage
COPY --from=builder /app/server/Cargo.lock /Cargo.lock

FROM ubuntu:24.04 AS runtime

RUN apt-get update && apt-get install -y \
    ca-certificates \
    libssl3 \
    libgomp1 \
    tini \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /home/ubuntu/.asr && chown -R ubuntu:ubuntu /home/ubuntu/
COPY --from=builder /usr/local/bin/asr-server /usr/local/bin/asr-server

ENV ORT_ARENA_CFG=cpu:0 \
    ORT_INTRA_OP_NUM_THREADS=1 \
    ORT_INTER_OP_NUM_THREADS=1 \
    OMP_NUM_THREADS=1

USER ubuntu
RUN id
WORKDIR /home/ubuntu

EXPOSE 9393

# use tini to passthrough Ctrl + C
ENTRYPOINT ["/usr/bin/tini", "--", "asr-server"]
