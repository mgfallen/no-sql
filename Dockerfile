# --- builder ---
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /app/app ./cmd/app

# --- runtime ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/app .

RUN adduser -D -u 1000 appuser
USER appuser

EXPOSE 8080

CMD ["./app"]