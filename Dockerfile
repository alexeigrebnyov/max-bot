# Этап сборки
FROM golang:1.25.0-alpine AS builder

WORKDIR /app

# Зависимости
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Собираем статически с небольшим бинарником
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o max-bot-service ./cmd

# Этап рантайма
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates

# Бинарник
COPY --from=builder /app/max-bot-service /app/max-bot-service
COPY --from=builder /app/web ./web

# Каталог для SQLite
RUN mkdir -p /app/data

ENV TZ=UTC

EXPOSE 9003

ENTRYPOINT ["/app/max-bot-service"]
