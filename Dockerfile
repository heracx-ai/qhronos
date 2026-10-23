# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.26.10 AS builder
WORKDIR /app
COPY . .
RUN make

# Run stage
FROM golang:1.26.10 AS hera-qhronos-production
WORKDIR /app
COPY --from=builder /app/bin/qhronosd .
COPY config.example.yml ./config.yml
EXPOSE 8081
ENTRYPOINT [ "./qhronosd", "--config", "./config.yml" ]