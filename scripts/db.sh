#!/bin/bash

# Configuration
CONTAINER_NAME="qhronos-postgres"
DB_USER="postgres"
DB_PASSWORD="postgres"
DB_NAME="qhronos"
DB_PORT="5432"
DATA_DIR="$(pwd)/docker/postgres/data"
IMAGE_NAME="qhronos-postgres:14"

# Function to build the custom image
build_image() {
    echo "Building custom PostgreSQL image with pg_cron support..."
    docker build -t $IMAGE_NAME -f docker/postgres/Dockerfile docker/postgres
}

# Function to start the database
start_db() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "PostgreSQL container is already running"
        return
    fi

    if [ "$(docker ps -aq -f name=$CONTAINER_NAME)" ]; then
        echo "Starting existing PostgreSQL container..."
        docker start $CONTAINER_NAME
    else
        echo "Creating and starting new PostgreSQL container..."
        docker run --name $CONTAINER_NAME \
            -e POSTGRES_USER=$DB_USER \
            -e POSTGRES_PASSWORD=$DB_PASSWORD \
            -e POSTGRES_DB=$DB_NAME \
            -p $DB_PORT:5432 \
            -v $DATA_DIR:/var/lib/postgresql/data \
            -d $IMAGE_NAME
    fi

    echo "Waiting for PostgreSQL to be ready..."
    until docker exec $CONTAINER_NAME pg_isready -U $DB_USER -d $DB_NAME; do
        sleep 1
    done

    echo "PostgreSQL is ready!"
}

# Function to stop the database
stop_db() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Stopping PostgreSQL container..."
        docker stop $CONTAINER_NAME
        echo "PostgreSQL container stopped"
    else
        echo "PostgreSQL container is not running"
    fi
}

# Function to remove the database container
remove_db() {
    stop_db
    if [ "$(docker ps -aq -f name=$CONTAINER_NAME)" ]; then
        echo "Removing PostgreSQL container..."
        docker rm $CONTAINER_NAME
        echo "PostgreSQL container removed"
    else
        echo "PostgreSQL container does not exist"
    fi
}

# Function to show database status
status_db() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "PostgreSQL container is running"
        docker exec $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -c "SELECT version();"
        docker exec $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -c "SELECT * FROM pg_extension WHERE extname = 'pg_cron';"
    else
        echo "PostgreSQL container is not running"
    fi
}

# Main script logic
case "$1" in
    start)
        build_image
        start_db
        ;;
    stop)
        stop_db
        ;;
    restart)
        stop_db
        start_db
        ;;
    remove)
        remove_db
        ;;
    status)
        status_db
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|remove|status}"
        exit 1
        ;;
esac 