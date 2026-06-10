# Build stage
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o morningstar-brain .

# Run stage
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/morningstar-brain .

EXPOSE 3000

CMD ["./morningstar-brain"]
