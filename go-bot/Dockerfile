# ---- Build stage ----
FROM golang:1.23-bookworm AS builder

WORKDIR /build

# Install ONNX Runtime 1.20.0
RUN wget -q https://github.com/microsoft/onnxruntime/releases/download/v1.20.0/onnxruntime-linux-x64-1.20.0.tgz \
    && tar -xzf onnxruntime-linux-x64-1.20.0.tgz \
    && cp onnxruntime-linux-x64-1.20.0/lib/* /usr/local/lib/ \
    && cp -r onnxruntime-linux-x64-1.20.0/include/* /usr/local/include/ \
    && ldconfig \
    && rm -rf onnxruntime-linux-x64-1.20.0 onnxruntime-linux-x64-1.20.0.tgz

# Install libtokenizers (HuggingFace tokenizer bindings for Go)
RUN wget -q https://github.com/daulet/tokenizers/releases/download/v1.26.0/libtokenizers.linux-amd64.tar.gz \
    && tar -xzf libtokenizers.linux-amd64.tar.gz -C /usr/local/lib/ \
    && ldconfig \
    && rm -f libtokenizers.linux-amd64.tar.gz

# Download Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and model files
COPY . .

# Build the bot binary
RUN CGO_ENABLED=1 CGO_LDFLAGS="-L/usr/local/lib" go build -tags ORT -o bot ./cmd/bot/

# ---- Runtime stage ----
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy ONNX Runtime shared libraries
COPY --from=builder /usr/local/lib/libonnxruntime* /usr/local/lib/
RUN ldconfig

# Copy the compiled binary
COPY --from=builder /build/bot /bot

# Copy model files
COPY --from=builder /build/model/ /model/

# Set environment variables
ENV MODEL_DIR=/model
ENV DATA_DIR=/data

CMD ["/bot"]
