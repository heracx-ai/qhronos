#!/bin/bash

set -e

CONTAINER_ID=$(docker ps -qf "name=qhronos_redis")
if [ -z "$CONTAINER_ID" ]; then
    echo "Redis container is not running. Please start it first using docker-compose up -d redis"
    exit 1
fi

echo "Flushing all Redis data in qhronos_redis..."
docker exec $CONTAINER_ID redis-cli FLUSHALL

echo "Redis cleanup complete." 