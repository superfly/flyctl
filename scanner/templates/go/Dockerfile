ARG GO_VERSION=1.22
ARG GO_BIN_NAME="main"

FROM golang:${GO_VERSION} as builder

# Repeated for multi-stage build
ARG GO_BIN_NAME

WORKDIR /app
COPY . /app

# Note: Fly.io uses linux, amd64
RUN GOOS=linux GOARCH=amd64 go build -o /app/${GO_BIN_NAME}



FROM debian:bookworm-slim

# Repeated for multi-stage build
ARG GO_BIN_NAME

COPY --from=builder /app/${GO_BIN_NAME} /usr/local/bin/${GO_BIN_NAME}
RUN ln -s /usr/local/bin/${GO_BIN_NAME} /usr/local/bin/run-app

CMD ["run-app"]
