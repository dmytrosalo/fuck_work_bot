# ---- Build stage ----
FROM golang:1.25-bookworm AS builder

WORKDIR /build

# Download Go dependencies
COPY go-bot/go.mod go-bot/go.sum ./
RUN go mod download

# Copy source code and model
COPY go-bot/ .

# Build the bot binary (no CGO needed)
RUN CGO_ENABLED=0 go build -o bot ./cmd/bot/

# ---- Runtime stage ----
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy the compiled binary
COPY --from=builder /build/bot /bot

# Copy model and seed data
COPY --from=builder /build/model/tfidf_model.json /model/tfidf_model.json
COPY --from=builder /build/seed_data.json /seed_data.json

ENV MODEL_PATH=/model/tfidf_model.json
ENV DATA_DIR=/data
ENV SEED_PATH=/seed_data.json

CMD ["/bot"]
