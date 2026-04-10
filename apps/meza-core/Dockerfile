FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY apps/meza-core/go.mod ./
COPY apps/meza-core/cmd ./cmd
COPY apps/meza-core/internal ./internal

RUN GOCACHE=/tmp/go-cache CGO_ENABLED=0 go build -o /out/meza-core ./cmd/api

FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache bash openssh-client
RUN adduser -D -H -s /sbin/nologin meza
RUN mkdir -p /data /data/ssh && chown -R meza:meza /data

COPY --from=builder /out/meza-core /usr/local/bin/meza-core

USER meza
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/meza-core"]
