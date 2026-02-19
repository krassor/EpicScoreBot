FROM golang:1.23-alpine as builder
RUN apk update && apk add --no-cache git && apk upgrade
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o bin/epicScoreBot app/main.go

FROM alpine:latest

RUN apk update && apk add --no-cache git && apk upgrade

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

COPY --from=builder /build/bin/epicScoreBot /bin/epicScoreBot

ENV HTTP_SERVER_PORT=8080
ENV HTTP_SERVER_ADDRESS_LISTEN=0.0.0.0

EXPOSE $HTTP_SERVER_PORT
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD []
