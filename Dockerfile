# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.23 AS builder
WORKDIR /app
COPY . .
RUN make

# Run stage
FROM golang:1.23
WORKDIR /app
COPY --from=builder /app/bin/qhronosd .
COPY config.example.yml ./config.yml
EXPOSE 8080
CMD ["./qhronosd", "-c", "config.yml"] 