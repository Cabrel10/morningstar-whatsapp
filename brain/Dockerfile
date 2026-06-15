# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o morningstar-brain .

# Run stage
FROM alpine:latest

WORKDIR /app

# Install dependencies for yt-dlp and ffmpeg
RUN apk add --no-cache \
    ca-certificates \
    ffmpeg \
    python3 \
    curl \
    && curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp

COPY --from=builder /app/morningstar-brain .

# Copy cookies if they exist (for yt-dlp YouTube auth)
COPY cookies.txt /app/cookies.txt

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3000/health || exit 1

CMD ["./morningstar-brain"]
