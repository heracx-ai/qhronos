# Qhronos: Scheduling and Reminder System

## Overview

**Qhronos** is a developer-first scheduling and notification service. It allows clients to schedule one-time or recurring events via API, store them durably, and manage them through a simple REST interface. The system is designed for reliability and simplicity, using PostgreSQL for persistence and Redis for scheduling.

---

## Value Proposition

- **Durable by Design**: PostgreSQL-backed event and occurrence tracking ensures no data loss.
- **Developer Experience First**: Simple JSON APIs with intuitive semantics.
- **Enterprise-Ready**: Role-based access control, token management, and audit trail.
- **Flexible Architecture**: Stateless components, scalable design, cloud or on-premise deployment.

---

## Key Features

- Create, update, delete scheduled events via REST API.
- Support for iCalendar (RFC 5545) recurrence rules.
- Role-based access control with JWT tokens.
- Separation of concerns between data storage and business logic.
- Audit trail via PostgreSQL.
- Reliable webhook delivery with retries.

---

## System Architecture

```plaintext
[Client/API Consumer]
       │
       ▼
   [API Service (Go)] ──────► [PostgreSQL]
       │                         │
       │                         └── stores: events, occurrences
       │
       ├─────► [Token Service]
       │              │
       │              └── manages: JWT tokens, access control
       │
       ├─────► [Redis]
       │         │
       │         ├── schedule:events (ZSET)
       │         └── recurring:events (HASH)
       │
       ├─────► [Recurring Event Expander]
       │              │
       │              └── expands recurring events into occurrences
       │
       └─────► [Dispatcher]
                      │
                      └── delivers webhooks with retries
```

---

## Data Model

### PostgreSQL

#### `events`

```sql
CREATE TABLE events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  description TEXT,
  start_time TIMESTAMPTZ NOT NULL,
  webhook_url TEXT NOT NULL,
  metadata JSONB DEFAULT '{}',
  schedule TEXT,
  tags TEXT[] DEFAULT '{}',
  status TEXT CHECK (status IN ('active', 'paused', 'deleted')) DEFAULT 'active',
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ
);
```

#### `occurrences`

```sql
CREATE TABLE occurrences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  scheduled_at TIMESTAMPTZ NOT NULL,
  status TEXT CHECK (status IN ('pending', 'scheduled', 'dispatched', 'completed', 'failed')) DEFAULT 'pending',
  last_attempt TIMESTAMPTZ,
  attempt_count INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT now()
);
```

---

## Authentication

Qhronos supports two authentication methods:

### 1. Master Token (Static API Key)

- Provided in header:
  ```http
  Authorization: Bearer <master_token>
  ```
- Grants full access to all endpoints without restriction.

### 2. JWT Token

- Provided in header:
  ```http
  Authorization: Bearer <jwt_token>
  ```
- JWT payload must include:
  ```json
  {
    "sub": "client_id_or_user_id",
    "access": "read" | "write" | "admin",
    "scope": ["user:rizqme", "system:tester"],
    "expires_at": "2024-05-01T00:00:00Z"
  }
  ```
- The `scope` field restricts access to events and occurrences that match the listed tags.
- Access is limited based on the `access` field:
  - `read`: can view events and occurrences within scope
  - `write`: can create/update/delete within scope
  - `admin`: full access to all data, bypassing scope restrictions

### Authentication Middleware

The API includes an authentication middleware that validates JWT tokens and enforces access control:

1. **Token Validation**:
   - Checks for presence of Authorization header
   - Validates token signature and expiration
   - Extracts and validates claims

2. **Access Control**:
   - Master tokens bypass all restrictions
   - JWT tokens are checked for:
     - Required access level
     - Required scope
     - Token expiration

3. **Error Responses**:
   - `401 Unauthorized`: Missing or invalid token
   - `403 Forbidden`: Insufficient access or scope
   - `400 Bad Request`: Invalid token format or claims

---

## Rate Limiting

Qhronos implements a token-bucket rate limiting algorithm backed by Redis to protect the API from abuse and ensure fair usage. The rate limiter is implemented as middleware and can be configured per route or globally.

### Implementation Details

1. **Token Bucket Algorithm**:
   - Uses Redis for distributed rate limiting
   - Implements atomic operations using Redis MULTI/EXEC
   - Supports token refill based on elapsed time
   - Configurable bucket size and refill rate

2. **Default Configuration**:
   - Bucket Size: 100 requests
   - Refill Rate: 10 requests per second
   - Window: 1 second
   - Key Format: `rate_limit:{token_id}`

3. **Client Identification**:
   - Primary: Token ID from JWT
   - Fallback: Client IP address
   - Supports per-token rate limiting

4. **Response Headers**:
   - `X-RateLimit-Limit`: Maximum requests per window
   - `X-RateLimit-Remaining`: Remaining requests
   - `X-RateLimit-Reset`: Time until reset
   - `Retry-After`: Wait time when rate limited

5. **Error Handling**:
   - `429 Too Many Requests`: Rate limit exceeded
   - `500 Internal Server Error`: Redis errors
   - Includes error details in response body

6. **Testing**:
   - Comprehensive test suite covering:
     - Basic rate limiting
     - Token refill
     - Per-token rate limiting
     - Configuration options
   - Uses test Redis database (DB 1)
   - Verifies headers and response codes

### Usage Example

```go
// Initialize rate limiter
rateLimiter := middleware.NewRateLimiter(redisClient,
    middleware.WithBucketSize(100),
    middleware.WithRefillRate(10),
    middleware.WithWindow(1),
)

// Apply to routes
router.Use(rateLimiter.RateLimit())
```

---

## API Design

### Events

#### Create Event

**POST** `/events`

Request:
```json
{
  "name": "Team Sync",
  "description": "Weekly team sync meeting",
  "start_time": "2025-05-01T09:00:00Z",
  "webhook_url": "https://app.com/hook",
  "metadata": {
    "meetingId": "abc123"
  },
  "schedule": "FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1",
  "tags": ["user:rizqme", "team:ai"]
}
```

Response:
```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "name": "Team Sync",
  "description": "Weekly team sync meeting",
  "start_time": "2025-05-01T09:00:00Z",
  "webhook_url": "https://app.com/hook",
  "metadata": {
    "meetingId": "abc123"
  },
  "schedule": "FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1",
  "tags": ["user:rizqme", "team:ai"],
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z"
}
```

#### Get Event

**GET** `/events/{id}`

Response:
```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "name": "Team Sync",
  "description": "Weekly team sync meeting",
  "start_time": "2025-05-01T09:00:00Z",
  "webhook_url": "https://app.com/hook",
  "metadata": {
    "meetingId": "abc123"
  },
  "schedule": "FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1",
  "tags": ["user:rizqme", "team:ai"],
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z",
  "updated_at": "2025-04-21T17:30:00Z"
}
```

#### Update Event

**PUT** `/events/{id}`

Request:
```json
{
  "name": "Updated Team Sync",
  "description": "Updated weekly team sync meeting",
  "schedule": "FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1",
  "tags": ["user:rizqme", "team:ai", "updated"]
}
```

Response:
```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "name": "Updated Team Sync",
  "description": "Updated weekly team sync meeting",
  "start_time": "2025-05-01T09:00:00Z",
  "webhook_url": "https://app.com/hook",
  "metadata": {
    "meetingId": "abc123"
  },
  "schedule": "FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1",
  "tags": ["user:rizqme", "team:ai", "updated"],
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z",
  "updated_at": "2025-04-21T17:30:00Z"
}
```

#### Delete Event

**DELETE** `/events/{id}`

Response: 200 OK

#### List Events

**GET** `/events?tags=user:rizqme,team:ai`

Response:
```json
[
  {
    "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
    "name": "Updated Team Sync",
    "description": "Updated weekly team sync meeting",
    "start_time": "2025-05-01T09:00:00Z",
    "webhook_url": "https://app.com/hook",
    "metadata": {
      "meetingId": "abc123"
    },
    "schedule": "FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1",
    "tags": ["user:rizqme", "team:ai", "updated"],
    "status": "active",
    "created_at": "2025-04-21T16:30:00Z",
    "updated_at": "2025-04-21T17:30:00Z"
  }
]
```

### Occurrences

#### Get Occurrence

**GET** `/occurrences/{id}`

Response:
```json
{
  "id": "25c5d4ba-9c2b-4824-9a7f-bb46d148d11b",
  "event_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "scheduled_at": "2025-04-22T00:00:00Z",
  "status": "pending",
  "last_attempt": null,
  "attempt_count": 0,
  "created_at": "2025-04-21T16:30:00Z"
}
```

#### List Occurrences

**GET** `/occurrences?tags=user:rizqme`

Response:
```json
[
  {
    "id": "25c5d4ba-9c2b-4824-9a7f-bb46d148d11b",
    "event_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
    "scheduled_at": "2025-04-22T00:00:00Z",
    "status": "pending",
    "last_attempt": null,
    "attempt_count": 0,
    "created_at": "2025-04-21T16:30:00Z"
  }
]
```

## Error Responses

The API uses standard HTTP status codes and returns error messages in the following format:

```json
{
  "error": "Error message description"
}
```

Common error codes:
- 400: Bad Request - Invalid input data
- 401: Unauthorized - Invalid or missing authentication
- 403: Forbidden - Insufficient permissions
- 404: Not Found - Resource not found
- 500: Internal Server Error - Server-side error

---

## Scheduler Logic

### Event Creation Flow

1. API receives event.
2. Stores metadata in `events` table.
3. If event has a schedule:
   - Adds to Redis: `HSET recurring:events <event_id> <event_data>`
4. Recurring Event Expander:
   - Runs every 5 minutes
   - Scans recurring events
   - Computes next 24 hours of occurrences
   - Adds to Redis: `ZADD schedule:events <timestamp> <occurrence_data>`

### Dispatcher Worker

1. Runs every second
2. Gets due events from Redis: `ZRANGEBYSCORE schedule:events 0 <now>`
3. For each occurrence:
   - Loads event from PostgreSQL
   - Attempts `POST` to `webhook_url`
   - On success: marks occurrence `dispatched`
   - On failure:
     - If retries < max: requeue with delay
     - Else: mark as `failed`

---

## Capacity Planning

### PostgreSQL
- Events table: ~1KB per event
- Occurrences table: ~500B per occurrence
- Estimated capacity: Millions of events and occurrences

### Redis
- Schedule events: ~400B per occurrence
- Recurring events: ~1KB per event
- 1GB Redis RAM → approx. 2.3 million occurrences

---

## Deployment Architecture

| Component         | Stack / Tool            |
| ----------------- | ----------------------- |
| API Backend       | Go (Gin)               |
| Postgres DB       | PostgreSQL 14+         |
| Scheduler Queue   | Redis 6+               |
| Recurrence Parser | `rrule-go`             |
| Containerization  | Docker / Kubernetes    |
| Monitoring        | Prometheus + Grafana   |
| Logging           | Loki or ELK Stack      |

---

## Status Endpoint

Expose a health and configuration overview via a **public** or **authenticated** endpoint.

**GET** `/status`

- **Auth**: optional for monitoring; if token provided, scope of response may be restricted.

**Response** 200 OK

```json
{
  "status": "ok",                  // overall health ("ok" | "degraded" | "down")
  "uptime_seconds": 86400,         // seconds since service start
  "version": "1.0.0",            // service version
  "jwt": {
    "issuer": "qhronos",
    "algorithm": "HS256",
    "default_exp_seconds": 3600,
    "token_info": {
      "sub": "rizqme",
      "access": "read_write",
      "scope": ["user:rizqme", "system:qa"],
      "exp": 1714569600
    }
  "hmac": {
    "algorithm": "HMAC-SHA256",
    "signature_header": "X-Qhronos-Signature",
    "default_secret": "qhronos.io",
    "event_override_supported": true
  }
```

- Clients may inspect the `hmac.event_override_supported` flag to know if individual events can specify a custom signing secret.

## Future Enhancements

- Webhook HMAC signing for validation
- Multi-tenant org/user support
- Admin dashboard for managing events
- Dead-letter queue management UI
- Rate-limited webhook batch dispatcher

---

## Project Structure

```
qhronos/
├── cmd/                    # Main application entry points
│   ├── api/
│   │   └── main.go
│   └── server/
│       └── main.go
├── internal/               # Private application code
│   ├── api/
│   │   └── routes.go
│   ├── config/
│   │   └── config.go
│   ├── database/
│   │   └── postgres.go
│   ├── handlers/
│   │   ├── event_handler.go
│   │   ├── event_handler_test.go
│   │   ├── occurrence_handler.go
│   │   ├── occurrence_handler_test.go
│   │   ├── status.go
│   │   ├── status_test.go
│   │   ├── token_handler.go
│   │   └── token_handler_test.go
│   ├── middleware/
│   │   ├── auth.go
│   │   ├── error_handler.go
│   │   ├── error_handler_test.go
│   │   ├── logging.go
│   │   ├── logging_test.go
│   │   ├── rate_limit.go
│   │   └── rate_limit_test.go
│   ├── models/
│   │   ├── errors.go
│   │   ├── event.go
│   │   ├── occurrence.go
│   │   └── token.go
│   ├── repository/
│   │   ├── event_repository.go
│   │   ├── event_repository_test.go
│   │   ├── occurrence_repository.go
│   │   └── occurrence_repository_test.go
│   ├── services/
│   │   ├── hmac_service.go
│   │   ├── hmac_service_test.go
│   │   ├── token_service.go
│   │   └── scheduler/
│   │       ├── cleanup.go
│   │       ├── cleanup_test.go
│   │       ├── dispatcher.go
│   │       ├── dispatcher_test.go
│   │       ├── expander.go
│   │       ├── expander_test.go
│   │       ├── schedule.go
│   │       ├── scheduler.go
│   │       └── scheduler_test.go
│   ├── testutils/
│   │   ├── db.go
│   │   ├── test_helpers.go
│   │   └── testutils.go
│   └── utils/              # Shared utilities (currently empty)
├── migrations/             # Database migration scripts
├── pkg/                    # Public packages (auth, database, redis)
│   ├── auth/
│   ├── database/
│   └── redis/
├── scripts/                # Utility scripts for development and ops
│   ├── apply_migrations.sh
│   ├── db.sh
│   ├── redis.sh
│   └── test.sh
├── config.example.yaml     # Example configuration
├── design.md               # This design document
├── docker-compose.yml      # Docker development environment
├── go.mod                  # Go module definition
├── go.sum                  # Go module checksums
├── LICENSE                 # Project license
└── README.md               # Project documentation
```

## Error Handling Patterns

### Repository Layer

The repository layer follows a consistent pattern for handling not found cases:

1. **Get Operations**:
   - When a resource is not found, return `(nil, nil)`
   - This applies to methods like `GetByID`, `GetByName`, etc.
   - Example:
     ```go
     event, err := repo.GetByID(ctx, id)
     if err != nil {
         return nil, err
     }
     if event == nil {
         return nil, nil  // Resource not found
     }
     ```

2. **Delete Operations**:
   - When deleting a non-existent resource, return `nil`
   - This applies to methods like `Delete`, `DeleteByID`, etc.
   - Example:
     ```go
     err := repo.Delete(ctx, id)
     if err != nil {
         return err
     }
     return nil  // Success, even if resource didn't exist
     ```

3. **Update Operations**:
   - When updating a non-existent resource, return `nil, nil`
   - This applies to methods like `Update`, `UpdateByID`, etc.
   - Example:
     ```go
     updated, err := repo.Update(ctx, id, update)
     if err != nil {
         return nil, err
     }
     if updated == nil {
         return nil, nil  // Resource not found
     }
     ```

This pattern ensures consistent behavior across all repositories and simplifies error handling in the service layer.

---

