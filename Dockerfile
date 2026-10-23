# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.26.10 AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 make && mkdir -p /app/logs

# Run stage
FROM gcr.io/distroless/static-debian12:nonroot AS hera-qhronos-production
WORKDIR /app
COPY --from=builder --chown=65532:65532 /app/bin/qhronosd ./qhronosd
COPY --from=builder --chown=65532:65532 /app/logs ./logs
COPY --chown=65532:65532 config.example.yml ./config.yml
EXPOSE 8081
ENTRYPOINT [ "./qhronosd", "--config", "./config.yml" ]