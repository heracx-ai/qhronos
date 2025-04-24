# Makefile for qhronosd

BINARY_NAME=bin/qhronosd

build:
	@mkdir -p ./bin
	go build -o $(BINARY_NAME) ./main.go

.PHONY: help build clean test migrate-up migrate-down docker-up docker-down run docker-build

help:
	@echo "Available targets:"
	@echo "  build        Build the qhronosd binary"
	@echo "  clean        Remove the qhronosd binary"
	@echo "  test         Run the test script (requires docker-up)"
	@echo "  migrate-up   Run migrations up using scripts/migrate.sh"
	@echo "  migrate-down Run migrations down using scripts/migrate.sh"
	@echo "  docker-up    Start postgres and redis containers"
	@echo "  docker-down  Stop all containers"
	@echo "  run          Build and run the qhronosd binary with default config"
	@echo "  docker-build Build the Docker image for qhronosd"

clean:
	rm -f $(BINARY_NAME)

test:
	bash scripts/test.sh

migrate-up:
	bash scripts/migrate.sh up

migrate-down:
	bash scripts/migrate.sh down

docker-up:
	docker-compose up -d postgres redis

docker-down:
	docker-compose down

run: build
	./$(BINARY_NAME) --config config.yaml

docker-build:
	docker build -t qhronosd:latest . 