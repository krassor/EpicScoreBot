# Этап, на котором выполняется сборка приложения
FROM golang:1.25-alpine AS builder
RUN apk update && apk add --no-cache git && apk upgrade
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o bin/epicScoreBot app/main.go

# Финальный этап, копируем собранное приложение
FROM alpine:latest

RUN apk update && apk add --no-cache git && apk upgrade

# Копируем скрипт entrypoint.sh в образ и делаем его исполняемым
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

COPY --from=builder /build/bin/epicScoreBot /bin/epicScoreBot 

ENV HTTP_SERVER_PORT=8080
ENV HTTP_SERVER_ADDRESS_LISTEN=0.0.0.0

EXPOSE $HTTP_SERVER_PORT
# Устанавливаем наш скрипт как точку входа
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
# CMD будет пустым, так как все управляется скриптом
CMD []