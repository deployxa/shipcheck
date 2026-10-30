# Multi-stage Dockerfile for Shipcheck CLI
FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/deployxa/shipcheck/internal/cli.Version=0.1.0-rc1" -o /bin/shipcheck ./cmd/shipcheck

FROM alpine:3.19

RUN apk --no-cache add ca-certificates git
COPY --from=builder /bin/shipcheck /usr/local/bin/shipcheck

USER nobody:nobody
ENTRYPOINT ["/usr/local/bin/shipcheck"]
CMD ["--help"]
