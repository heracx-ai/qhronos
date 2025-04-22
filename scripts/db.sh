#!/bin/bash

DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=qhronos
CONTAINER_NAME=qhronos_db
PORT=5433

function build() {
    echo "Starting PostgreSQL container..."
    docker-compose up -d postgres
}

function start() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Container is already running"
    else
        if [ "$(docker ps -aq -f status=exited -f name=$CONTAINER_NAME)" ]; then
            docker start $CONTAINER_NAME
            echo "Container started"
        else
            build
        fi
    fi
}

function stop() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        docker stop $CONTAINER_NAME
        echo "Container stopped"
    else
        echo "Container is not running"
    fi
}

function restart() {
    stop
    start
}

function remove() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        docker stop $CONTAINER_NAME
    fi
    if [ "$(docker ps -aq -f name=$CONTAINER_NAME)" ]; then
        docker rm $CONTAINER_NAME
        echo "Container removed"
    else
        echo "Container does not exist"
    fi
}

function status() {
    if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
        echo "Container is running"
        docker ps -f name=$CONTAINER_NAME
        echo "Database connection info:"
        echo "  Host: localhost"
        echo "  Port: $PORT"
        echo "  User: $DB_USER"
        echo "  Password: $DB_PASSWORD"
        echo "  Database: $DB_NAME"
    else
        if [ "$(docker ps -aq -f status=exited -f name=$CONTAINER_NAME)" ]; then
            echo "Container exists but is not running"
        else
            echo "Container does not exist"
        fi
    fi
}

case "$1" in
    "build")
        build
        ;;
    "start")
        start
        ;;
    "stop")
        stop
        ;;
    "restart")
        restart
        ;;
    "remove")
        remove
        ;;
    "status")
        status
        ;;
    *)
        echo "Usage: $0 {build|start|stop|restart|remove|status}"
        exit 1
        ;;
esac