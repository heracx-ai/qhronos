# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.23 as builder
WORKDIR /app
COPY . .
RUN go build -o qhronosd ./cmd

# Run stage
FROM debian:bullseye-slim
WORKDIR /app
COPY --from=builder /app/qhronosd .
COPY config.example.yaml ./config.yaml
EXPOSE 8080
CMD ["./qhronosd"] 