# --- Build stage ---
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS builder
WORKDIR /app

ENV PATH="/go/bin:$PATH"
RUN apk add --no-cache make git curl && \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b /usr/local/bin && \
    go install honnef.co/go/tools/cmd/staticcheck@latest
COPY . .
RUN make build-nocheck && \
    CGO_ENABLED=0 GOOS=linux go build -o /usr/bin/healthcheck ./healthcheck/main.go

# --- Final stage: scratch ---
FROM scratch
WORKDIR /
ARG TARGETPLATFORM

COPY --from=builder /app/bin/modbusctl /bin/modbusctl
COPY --from=builder /usr/bin/healthcheck /bin/healthcheck

VOLUME ["/captures"]

ENTRYPOINT ["/bin/modbusctl", "server"]