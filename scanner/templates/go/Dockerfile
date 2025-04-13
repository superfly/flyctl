ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm as builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -o /run-app .


FROM debian:bookworm

COPY --from=builder /run-app /usr/local/bin/

# Add other copy statements as needed, for example:
# COPY --from=builder /usr/src/app/templates /usr/src/app/templates

CMD ["run-app"]
