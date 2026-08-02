# Dockerfile for telegram-mpd-bot
# - Go 1.21 build stage, statically linked binary (CGO_ENABLED=0)
# - Runtime: Alpine + MPD running locally inside the container
# - entrypoint.sh starts mpd, waits for its socket, then execs the bot

# ---------- Build stage ----------
FROM golang:1.21-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o bot ./cmd/bot/main.go

# ---------- Runtime stage ----------
FROM alpine:3.18

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    mpd \
    mpc \
    ffmpeg \
    su-exec

RUN addgroup -S bot && adduser -S bot -G bot

WORKDIR /app

COPY --from=builder /build/bot .
COPY config/mpd.conf /etc/mpd.conf
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Volumes: bot config, MPD state (includes music, playlists, db — all in one
# place now since the bot writes converted tracks directly into MPD's library)
VOLUME ["/app/config", "/var/lib/mpd"]

# MPD's own socket dir needs to be writable at runtime; entrypoint fixes ownership
EXPOSE 6600 8000

ENTRYPOINT ["/entrypoint.sh"]