# --- Build stage ---
FROM golang:1.24.13-alpine AS builder
WORKDIR /app

RUN apk add --no-cache make git curl && \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b /usr/local/bin
COPY . .
RUN make build && \
    CGO_ENABLED=0 GOOS=linux go build -o /usr/bin/healthcheck ./healthcheck/main.go

# --- Final stage: scratch ---
FROM scratch
WORKDIR /
ARG TARGETPLATFORM

COPY --from=builder /app/modbusctl /bin/modbusctl
COPY --from=builder /usr/bin/healthcheck /bin/healthcheck

VOLUME ["/mcap"]

ENTRYPOINT ["/bin/modbusctl", "server"]