# Qhronos: Reliable Scheduling & Webhook Delivery

![Version](https://img.shields.io/badge/version-0.1.0-blue)
[![CI](https://github.com/feedloop/qhronos/actions/workflows/ci.yml/badge.svg)](https://github.com/feedloop/qhronos/actions/workflows/ci.yml)

## Table of Contents
- [Overview](#overview)
- [Features](#features)
- [Quick Start](#quick-start)
- [API Usage](#api-usage)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [Authentication](#authentication)
- [Contributing](#contributing)
- [Testing](#testing)
- [Troubleshooting & FAQ](#troubleshooting--faq)
- [License](#license)
- [Links & References](#links--references)

## API Documentation

See [docs/api.md](docs/api.md) for full API details and example requests/responses.

## Overview
Qhronos (v0.1.0) is a developer-first scheduling and notification platform. It lets you schedule one-time or recurring events and reliably delivers webhooks at the right time. Built for reliability, security, and extensibility, Qhronos is ideal for automating workflows, orchestrating AI agents, and managing time-based triggers in distributed systems.

For system architecture and in-depth design, see [design.md](./design.md).

## Features
- REST API for event scheduling
- Recurring and one-time events (iCalendar RFC 5545)
- Reliable, retryable webhook delivery
- JWT and master token authentication
- Rate limiting and audit logging
- Easy deployment (Docker/Kubernetes)

## Quick Start

### Prerequisites
- Go 1.20+
- Docker & Docker Compose
- PostgreSQL & Redis (or use Docker Compose)

### Run with Docker Compose
```sh
git clone https://github.com/feedloop/qhronos.git
cd qhronos
docker-compose up
```
The API will be available at `http://localhost:8080`.

### Build and Run from Binary
```sh
make build
./bin/qhronosd --config config.yaml
```
You can use CLI flags to override config values, e.g.:
```sh
./bin/qhronosd --port 9090 --log-level debug
```

### Configuration Setup
Before running Qhronos, set up your configuration:

1. Copy the example config file:
   ```sh
   cp config.example.yaml config.yaml
   ```
2. Edit `config.yaml` to match your environment (database, Redis, auth secrets, etc).
3. You can also override any config value using CLI flags (see above).

### Database Setup & Migration
Before running Qhronos for the first time, initialize the PostgreSQL database and apply migrations:

1. Start PostgreSQL and Redis using Docker Compose:
   ```sh
   docker-compose up -d postgres redis
   ```
2. Run database migrations using the binary:
   ```sh
   ./bin/qhronosd --migrate --config config.yaml
   ```

This will create all required tables and schema in your database.

## Database Migration

Qhronos manages database schema changes using embedded migration files. You can apply all migrations using:

```sh
./bin/qhronosd --migrate --config config.yaml
```

- This will apply all pending migrations to your database.
- You can use any CLI flags to override config values (e.g., DB host, port, user).

### Notes
- Migration files are embedded in the binary; no external migration tool is needed.
- The binary manages a `schema_migrations` table to track applied migrations.
- Migration files should be named with incremental prefixes (e.g., `001_initial_schema.sql`, `002_add_table.sql`, etc.).

## API Usage
See [API documentation](docs/api.md) for full details.

**Create an event:**
```sh
curl -X POST http://localhost:8080/events \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Daily Backup",
    "start_time": "2024-03-20T00:00:00Z",
    "webhook_url": "https://example.com/webhook",
    "schedule": {
      "frequency": "weekly",
      "interval": 1,
      "by_day": ["MO", "WE", "FR"]
    },
    "tags": ["system:backup"]
  }'
```

## Schedule Parameter Tutorial

The `schedule` parameter in event creation allows you to define recurring or one-time schedules using a flexible JSON structure. Here are the most common use cases:

### 1. One-Time Event (No Recurrence)
If you omit the `schedule` field, the event will be scheduled only once at the specified `start_time`:
```json
{
  "name": "One-Time Event",
  "description": "This event happens only once.",
  "start_time": "2024-05-01T10:00:00Z",
  "webhook_url": "https://example.com/webhook",
  "metadata": {},
  "tags": ["single"]
  // No "schedule" field!
}
```

### 2. Daily Recurring Event
```json
"schedule": {
  "frequency": "daily"
}
```
This schedules the event to occur every day. (If `interval` is omitted, it defaults to 1.)

### 3. Weekly Recurring Event (e.g., every Monday and Friday)
```json
"schedule": {
  "frequency": "weekly",
  "by_day": ["MO", "FR"]
}
```
This schedules the event to occur every Monday and Friday.

**Tip:** Omitting the `schedule` field results in a one-time event. For recurring events, specify the `schedule` field as shown above.

## Configuration
- Copy `config.example.yaml` to `config.yaml` and edit as needed.
- Or set CLI flags to override config values.

### Scheduler Lookahead Settings

The scheduler section in your config controls how far into the future recurring events are expanded and how often the expander runs:

```yaml
scheduler:
  look_ahead_duration: 24h   # How far into the future to expand recurring events
  expansion_interval: 5m     # How often to run the expander
```
- `look_ahead_duration`: Controls the window (e.g., 24h) for which recurring event occurrences are pre-generated.
- `expansion_interval`: How frequently the expander checks and generates new occurrences.

Adjust these values in your `config.yaml` to tune scheduling behavior for your workload.

## Deployment
- Supports Docker, Docker Compose, and Kubernetes.
- See [deployment guide](docs/deployment.md) for production tips.

## Docker Usage

You can build and run Qhronos using Docker:

### Build the Docker image
```sh
make docker-build
# or
# docker build -t qhronosd:latest .
```

### Run the Docker container
```sh
docker run -p 8080:8080 qhronosd:latest
```

### Override configuration
- **Custom config file:**
  ```sh
  docker run -v /path/to/your/config.yaml:/app/config.yaml -p 8080:8080 qhronosd:latest
  ```
- **CLI flags (recommended):**
  ```sh
  docker run qhronosd:latest --port 9090 --log-level debug
  ```

You can combine these methods as needed.

## Authentication
- Use a master token or generate JWTs via the `/tokens` endpoint.
- See [docs/auth.md](docs/auth.md) for details.

### JWT Tokens
Qhronos supports JWT (JSON Web Token) authentication for secure, scoped API access.

- **Obtaining a JWT Token:**
  Use your master token to request a JWT via the `/tokens` endpoint:
  ```sh
  curl -X POST http://localhost:8080/tokens \
    -H "Authorization: Bearer <master_token>" \
    -H "Content-Type: application/json" \
    -d '{
      "sub": "your-user-id",
      "access": "admin",
      "scope": ["user:your-username"],
      "expires_at": "2024-12-31T23:59:59Z"
    }'
  ```