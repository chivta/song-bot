# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS dev
WORKDIR /app
RUN go install github.com/air-verse/air@latest
COPY go.mod go.sum ./
RUN go mod download
CMD ["air", "-c", ".air.toml"]

FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/songbot ./cmd/songbot

FROM alpine:3.21 AS production
# ffmpeg and ffprobe come from Alpine, not from go-ytdlp's downloader: the
# builds it fetches are glibc-linked with no musl variant published, so they
# cannot exec here. yt-dlp itself is still downloaded at runtime, since it does
# ship a musl build. libc6-compat covers the yt-dlp glibc fallback path in case
# musl detection ever misfires.
RUN apk add --no-cache ca-certificates wget libc6-compat ffmpeg && adduser -D -u 10001 songbot
WORKDIR /app
COPY --from=builder /out/songbot /app/songbot
# The state volume mounts here; owning it up front keeps the container writable
# even where fsGroup is not honoured.
RUN mkdir -p /app/data && chown 10001:10001 /app/data
USER 10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -q --spider http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/songbot"]
