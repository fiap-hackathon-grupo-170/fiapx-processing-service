# ---- Stage 1: Build ----
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/worker ./cmd/worker

# ---- Stage 2: Runtime ----
FROM alpine:3.19

RUN apk add --no-cache ffmpeg ca-certificates tzdata \
    && adduser -D -u 1000 appuser

WORKDIR /app
COPY --from=builder /app/worker .
COPY migrations/ ./migrations/

RUN mkdir -p /tmp/fiapx && chown appuser:appuser /tmp/fiapx

USER appuser

EXPOSE 8083

ENTRYPOINT ["./worker"]
