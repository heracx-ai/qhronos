# Qhronos: Scheduling and Reminder System

## Overview

**Qhronos** is a developer-first scheduling and notification service. It allows clients to schedule one-time or recurring events via API, store them durably, and manage them through a simple REST interface. The system is designed for reliability and simplicity, using PostgreSQL for persistence.

---

## Value Proposition

- **Durable by Design**: PostgreSQL-backed event and occurrence tracking ensures no data loss.
- **Developer Experience First**: Simple JSON APIs with intuitive semantics.
- **Enterprise-Ready**: Role-based access control, token management, and audit trail.
- **Flexible Architecture**: Stateless components, scalable design, cloud or on-premise deployment.

---

## Key Features

- Create, update, delete scheduled events via REST API.
- Support for cron-like schedule expressions.
- Role-based access control with JWT tokens.
- Separation of concerns between data storage and business logic.
- Audit trail via PostgreSQL.

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
       └─────► [Token Service]
                      │
                      └── manages: JWT tokens, access control
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
  schedule TEXT NOT NULL,
  tags TEXT[] DEFAULT '{}',
  metadata JSONB DEFAULT '{}',
  status TEXT CHECK (status IN ('active', 'deleted')) DEFAULT 'active',
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

#### `occurrences`

```sql
CREATE TABLE occurrences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id UUID REFERENCES events(id) ON DELETE CASCADE,
  scheduled_at TIMESTAMPTZ NOT NULL,
  status TEXT CHECK (status IN ('pending', 'completed', 'failed')) DEFAULT 'pending',
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX ON occurrences(scheduled_at) WHERE status = 'pending';
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

---

## Token Management

Qhronos uses a single, static **Master Token** configured via the environment variable `QHRONOS_MASTER_TOKEN` at service startup. This token:

- Cannot be changed or revoked at runtime.
- Grants full access to all API endpoints, bypassing scope and access restrictions.

### JWT Token Issuance

- **Endpoint**
  ```http
  POST /tokens
  Authorization: Bearer <master_token>
  ```
- **Request Body**
  ```json
  {
    "type": "jwt",
    "sub": "target_user_id",
    "access": "read" | "write" | "admin",
    "scope": ["user:rizqme", "system:tester"],
    "expires_at": "2024-05-01T00:00:00Z"
  }
  ```
- **Response**
  ```json
  {
    "token": "<new_jwt_token>",
    "type": "jwt",
    "sub": "target_user_id",
    "access": "read",
    "scope": ["user:rizqme", "system:tester"],
    "expires_at": "2024-05-01T00:00:00Z"
  }
  ```

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

## API Design

### Create Event

**POST** `/events`

**Request**

```json
{
  "name": "Team Sync",
  "description": "Weekly team sync meeting",
  "schedule": "0 0 * * *",
  "tags": ["user:rizqme", "team:ai"],
  "metadata": {
    "meetingId": "abc123"
  }
}
```

**Response**

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "name": "Team Sync",
  "description": "Weekly team sync meeting",
  "schedule": "0 0 * * *",
  "tags": ["user:rizqme", "team:ai"],
  "metadata": {
    "meetingId": "abc123"
  },
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z",
  "updated_at": "2025-04-21T16:30:00Z"
}
```

### Update Event

**PUT** `/events/{id}`

**Request**

```json
{
  "name": "Updated Meeting",
  "description": "Updated description",
  "schedule": "0 0 * * *",
  "tags": ["user:rizqme", "team:ai", "updated"],
  "metadata": {
    "meetingId": "abc123",
    "newField": "value"
  }
}
```

**Response**

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "name": "Updated Meeting",
  "description": "Updated description",
  "schedule": "0 0 * * *",
  "tags": ["user:rizqme", "team:ai", "updated"],
  "metadata": {
    "meetingId": "abc123",
    "newField": "value"
  },
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z",
  "updated_at": "2025-04-21T17:30:00Z"
}
```

### Get Event

**GET** `/events/{id}`

**Response**

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "name": "Updated Meeting",
  "description": "Updated description",
  "schedule": "0 0 * * *",
  "tags": ["user:rizqme", "team:ai", "updated"],
  "metadata": {
    "meetingId": "abc123",
    "newField": "value"
  },
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z",
  "updated_at": "2025-04-21T17:30:00Z"
}
```

### Delete Event

**DELETE** `/events/{id}`

**Response**: 200 OK

### List Events

**GET** `/events?tags=user:rizqme,team:ai`

**Response**

```json
[
  {
    "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
    "name": "Updated Meeting",
    "description": "Updated description",
    "schedule": "0 0 * * *",
    "tags": ["user:rizqme", "team:ai", "updated"],
    "metadata": {
      "meetingId": "abc123",
      "newField": "value"
    },
    "status": "active",
    "created_at": "2025-04-21T16:30:00Z",
    "updated_at": "2025-04-21T17:30:00Z"
  }
]
```

### Get Occurrence

**GET** `/occurrences/{id}`

**Response**

```json
{
  "id": "25c5d4ba-9c2b-4824-9a7f-bb46d148d11b",
  "event_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "scheduled_at": "2025-04-22T00:00:00Z",
  "status": "pending",
  "created_at": "2025-04-21T16:30:00Z",
  "updated_at": "2025-04-21T16:30:00Z"
}
```

### List Occurrences

**GET** `/occurrences?tags=user:rizqme`

**Response**

```json
[
  {
    "id": "25c5d4ba-9c2b-4824-9a7f-bb46d148d11b",
    "event_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
    "scheduled_at": "2025-04-22T00:00:00Z",
    "status": "pending",
    "created_at": "2025-04-21T16:30:00Z",
    "updated_at": "2025-04-21T16:30:00Z"
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
3. Computes first occurrence (or multiple) using `rrule`.
4. Inserts into `occurrences`.
5. Adds to Redis: `ZADD schedule:events <timestamp> <occurrence_id>`.

### Recurring Expander (Cron)

1. Runs every 5–15 minutes.
2. Scans recurring events.
3. Checks for the last expanded time.
4. Computes next batch of occurrences.
5. Inserts into `occurrences`, `ZADD` into Redis.

### Scheduler Worker (Go)

```go
for {
  now := time.Now().Unix()
  // Use ZRANGEBYSCORE with LIMIT or ZPOPMIN
  due, err := redis.ZRangeByScoreWithScores(ctx, "schedule:events", &redis.ZRangeBy{
    Min: "-inf",
    Max: fmt.Sprintf("%d", now),
    Count: 100,
  }).Result()

  if err != nil || len(due) == 0 {
    time.Sleep(500 * time.Millisecond)
    continue
  }

  for _, item := range due {
    dispatchChan <- item.Member.(string)
    redis.ZRem(ctx, "schedule:events", item.Member)
  }
}
```

### Dispatcher Worker (Go)

1. Loads event + occurrence from PostgreSQL.
2. Attempts `POST` to `webhook_url`.
3. On success: marks occurrence `dispatched`.
4. On failure:
   - If retries < max: requeue with delay
   - Else: mark as `failed`

---

## Capacity Planning (Redis)

Assuming:

- 400 bytes per event in Redis
- 1GB Redis RAM → approx. **2.3 million occurrences** (best-effort estimate)

---

## Deployment Architecture

| Component         | Stack / Tool            |
| ----------------- | ----------------------- |
| API Backend       | Go (Gin / Echo / Fiber) |
| Postgres DB       | PostgreSQL 14+          |
| Scheduler Queue   | Redis 6+ (ZSET)         |
| Recurrence Parser | `rrule-go`              |
| Containerization  | Docker / Kubernetes     |
| Monitoring        | Prometheus + Grafana    |
| Logging           | Loki or ELK Stack       |

---

## Rate Limiting

To protect Qhronos from abuse and ensure fair usage, the API enforces per-token rate limits using a **token-bucket** algorithm backed by Redis.

- **Key**: `rate_limit:{token_id}`
- **Algorithm**: Token Bucket
  - **Refill rate (********`R`********\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*)**: tokens added per second (configurable)
  - **Bucket capacity (********`B`********\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*)**: maximum burst size (configurable)
- **Default Quotas**:
  - **Master Token**: `R=1000 requests/minute`, `B=200`
  - **Admin-level JWT**: `R=500 requests/minute`, `B=100`
  - **Read/Write JWT**: `R=200 requests/minute`, `B=50`
- **Enforcement Flow**:
  1. On each incoming request, run an atomic Redis Lua script that:
     - Calculates tokens to add since last check (`elapsed_seconds * R`).
     - Increases current bucket count (capped at `B`).
     - If bucket ≥ 1, decrement by 1 and allow the request.
     - If bucket < 1, deny with `429 Too Many Requests` and `Retry-After` header set to the time until next token is available.
- **Configuration**:
  - Environment variables:
    - `RATE_LIMIT_MASTER_R`, `RATE_LIMIT_MASTER_B`
    - `RATE_LIMIT_ADMIN_R`, `RATE_LIMIT_ADMIN_B`
    - `RATE_LIMIT_RW_R`, `RATE_LIMIT_RW_B`
  - Defaults can be tuned per deployment.
- **Monitoring**:
  - Expose Prometheus metrics:
    - `qhronos_rate_limit_allowed_total{token_type="$type"}`
    - `qhronos_rate_limit_denied_total{token_type="$type"}`
    - `qhronos_rate_limit_current_bucket{token_type="$type"}`

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

