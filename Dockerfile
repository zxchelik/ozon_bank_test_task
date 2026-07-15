FROM golang:1.25.7-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/shortener ./cmd/shortener

FROM golang:1.25.7-alpine AS goose-builder

ARG GOOSE_VERSION=v3.27.1
RUN CGO_ENABLED=0 go install -tags='no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}

FROM alpine:3.21 AS migrator

RUN addgroup -S goose && adduser -S -G goose goose
COPY --from=goose-builder /go/bin/goose /usr/local/bin/goose
COPY migrations /migrations
ENV GOOSE_DRIVER=postgres
ENV GOOSE_MIGRATION_DIR=/migrations
USER goose
ENTRYPOINT ["/usr/local/bin/goose"]

FROM alpine:3.21 AS runtime

RUN addgroup -S shortener && adduser -S -G shortener shortener
COPY --from=builder /out/shortener /usr/local/bin/shortener
USER shortener
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/shortener"]
