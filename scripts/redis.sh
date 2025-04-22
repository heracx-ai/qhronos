#!/bin/bash

# Configuration
CONTAINER_NAME="qhronos-redis"
REDIS_PORT="6379"
DATA_DIR="$(pwd)/docker/redis/data"

# Function to start Redis
start_redis() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Redis container is already running"
        return
    fi

    if [ "$(docker ps -aq -f name=$CONTAINER_NAME)" ]; then
        echo "Starting existing Redis container..."
        docker start $CONTAINER_NAME
    else
        echo "Creating and starting new Redis container..."
        docker run --name $CONTAINER_NAME \
            -p $REDIS_PORT:6379 \
            -v $DATA_DIR:/data \
            -d redis:7 \
            redis-server --appendonly yes
    fi

    echo "Waiting for Redis to be ready..."
    until docker exec $CONTAINER_NAME redis-cli ping > /dev/null 2>&1; do
        sleep 1
    done

    echo "Redis is ready!"
}

# Function to stop Redis
stop_redis() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Stopping Redis container..."
        docker stop $CONTAINER_NAME
        echo "Redis container stopped"
    else
        echo "Redis container is not running"
    fi
}

# Function to remove Redis container
remove_redis() {
    stop_redis
    if [ "$(docker ps -aq -f name=$CONTAINER_NAME)" ]; then
        echo "Removing Redis container..."
        docker rm $CONTAINER_NAME
        echo "Redis container removed"
    else
        echo "Redis container does not exist"
    fi
}

# Function to show Redis status
status_redis() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Redis container is running"
        docker exec $CONTAINER_NAME redis-cli info server | grep "redis_version"
    else
        echo "Redis container is not running"
    fi
}

# Function to connect to Redis CLI
cli_redis() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        docker exec -it $CONTAINER_NAME redis-cli
    else
        echo "Redis container is not running"
    fi
}

# Main script logic
case "$1" in
    start)
        start_redis
        ;;
    stop)
        stop_redis
        ;;
    restart)
        stop_redis
        start_redis
        ;;
    remove)
        remove_redis
        ;;
    status)
        status_redis
        ;;
    cli)
        cli_redis
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|remove|status|cli}"
        exit 1
        ;;
esac 