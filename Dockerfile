# ---------- Builder ----------
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o app ./cmd/app

# ---------- Runtime ----------
FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/app .

CMD ["./app"]