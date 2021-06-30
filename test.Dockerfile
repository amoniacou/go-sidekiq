FROM golang:1.16-alpine
RUN apk add --no-cache git make build-base
WORKDIR $GOPATH/src/github.com/amoniacou/go-sidekiq
