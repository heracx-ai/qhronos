# Qhronos: Scheduling and Reminder System

## Overview

**Qhronos** is a developer-first scheduling and webhook reminder service. It allows clients to schedule one-time or recurring events via API, store them durably, and trigger HTTP webhooks precisely when the time arrives. The system is designed for reliability, scalability, and simplicity, using PostgreSQL for persistence and Redis for high-speed dispatching.

---

## Value Proposition

- **Durable by Design**: PostgreSQL-backed event and occurrence tracking ensures no data loss.
- **High-Performance Scheduling**: Redis sorted sets enable millisecond-precision dispatching at scale.
- **Developer Experience First**: Simple JSON APIs with intuitive semantics.
- **Enterprise-Ready**: Recurring schedules, retries, HMAC signing, observability, and dead-letter queues.
- **Flexible Architecture**: Stateless components, scalable workers, cloud or on-premise deployment.

---

## Key Features

- Create, update, delete scheduled events via REST API.
- Support for RFC-5545-based recurring events (e.g., weekly, every 3rd Thursday).
- Reliable webhook delivery with configurable retries.
- Separation of data durability (PostgreSQL) and dispatch performance (Redis).
- Audit trail and metrics via Postgres and Prometheus.

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
       └─────► [Redis (ZSET)]
                      ▲
                      │
       [Recurring Event Expander] (Go, cron job)
                      │
                      ▼
            [Scheduler Worker(s) in Go]
                      │
                      ▼
              [Webhook Dispatcher (Go)]
                      │
                      ▼
             HTTP POST → External System
```

---

## Data Model

### PostgreSQL

#### `events`

```sql
CREATE TABLE events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL,
  start_time TIMESTAMPTZ NOT NULL,
  webhook_url TEXT NOT NULL,
  payload JSONB NOT NULL,
  recurrence TEXT NULL, -- RFC 5545
  tags TEXT[] DEFAULT '{}',
  status TEXT CHECK (status IN ('active', 'paused', 'deleted')) DEFAULT 'active',
  created_at TIMESTAMPTZ DEFAULT now()
);
```

#### `occurrences`

```sql
CREATE TABLE occurrences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id UUID REFERENCES events(id) ON DELETE CASCADE,
  scheduled_at TIMESTAMPTZ NOT NULL,
  status TEXT CHECK (status IN ('scheduled', 'dispatched', 'failed')) DEFAULT 'scheduled',
  last_attempt TIMESTAMPTZ,
  attempt_count INT DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX ON occurrences(scheduled_at) WHERE status = 'scheduled';
```

---

## Redis Structure

- **Sorted Set**: `schedule:events`
  - Member: `occurrence_id`
  - Score: UNIX timestamp (ms or s) for scheduled time

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
    "exp": 1714569600,
    "access": "read" | "write" | "read_write" | "admin",
    "scope": ["user:rizqme", "system:tester"]
  }
  ```
- The `scope` field restricts access to events and occurrences that match **all** listed tags.
- Access is limited based on the `access` field:
  - `read`: can view events and occurrences within scope
  - `write`: can create/update/delete within scope
  - `read_write`: both read and write access
  - `admin`: full access to all data, bypassing scope restrictionshttp Authorization: Bearer \<jwt\_token>
  ```
  ```
- JWT payload must include:
  ```json
  {
    "sub": "client_id_or_user_id",
    "exp": 1714569600,
    "access": "read" | "write" | "read_write",
    "scope": ["user:rizqme", "system:tester"]
  }
  ```
- The `scope` field restricts access to events and occurrences that match **all** listed tags.
- Access is limited based on the `access` field (read-only, write-only, or both).

---

## Token Management

Qhronos uses a single, static **Master Token** configured via the environment variable `QHRONOS_MASTER_TOKEN` at service startup. This token:

- Cannot be changed or revoked at runtime.
- Grants full access to all API endpoints, bypassing scope and access restrictions.

### JWT Token Issuance

- **Endpoint**
  ```http
  POST /create_token
  Authorization: Bearer <master_token> or Bearer <jwt_token>
  ```
- **Request Body**
  ```json
  {
    "type": "jwt",
    "sub": "target_user_id",
    "exp": 1714569600,
    "access": "read" | "write" | "read_write" | "admin",
    "scope": ["user:rizqme", "system:tester"]
  }
  ```
- **Issuance Rules**
  1. **Master Token Authentication**
     - No restrictions: can issue JWTs for any `sub`, any `access`, any `scope`.
  2. **JWT Authentication**
     - **Admin-level JWT** (`access = "admin"`):
       - Can issue JWTs for any `sub` and any `access` level, **but** the `scope` of the new token **must** be a subset of the issuer's `scope` (cannot create a superset of tags).
     - **Non-admin JWT**:
       - `sub` must equal the issuer's `sub` claim.
       - `access` must be the same or lower privilege than the issuer's (order: `read` < `write` < `read_write`).
       - `scope` must be a subset of the issuer's `scope`.
- **Response**
  ```json
  {
    "token": "<new_jwt_token>",
    "type": "jwt",
    "sub": "target_user_id",
    "access": "read",
    "scope": ["user:rizqme", "system:tester"],
    "exp": 1714569600
  }
  ```

---

## API Design

### Create Event

**POST** `/events`

**Request**

```json
{
  "title": "Team Sync",
  "start_time": "2025-05-01T09:00:00Z",
  "webhook_url": "https://app.com/hook",
  "payload": {
    "meetingId": "abc123"
  },
  "tags": ["user:rizqme", "team:ai"],
  "recurrence": "FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"
}
```

**Response**

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "title": "Team Sync",
  "start_time": "2025-05-01T09:00:00Z",
  "webhook_url": "https://app.com/hook",
  "payload": {
    "meetingId": "abc123"
  },
  "tags": ["user:rizqme", "team:ai"],
  "recurrence": "FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1",
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z"
}
```

### Update Event

**PUT** `/events/{id}`

**Request**

```json
{
  "title": "Updated Meeting",
  "start_time": "2025-05-01T10:00:00Z",
  "recurrence": null,
  "status": "paused"
}
```

**Response**

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "title": "Updated Meeting",
  "start_time": "2025-05-01T10:00:00Z",
  "webhook_url": "https://app.com/hook",
  "payload": {
    "meetingId": "abc123"
  },
  "recurrence": null,
  "status": "paused",
  "created_at": "2025-04-21T16:30:00Z",
  "updated_at": "2025-04-22T08:10:00Z"
}
```

### Get Event

**GET** `/events/{id}`

**Response**

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "title": "Team Sync",
  "start_time": "2025-05-01T09:00:00Z",
  "webhook_url": "https://app.com/hook",
  "payload": {
    "meetingId": "abc123"
  },
  "recurrence": "FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1",
  "tags": ["user:rizqme", "team:ai"],
  "status": "active",
  "created_at": "2025-04-21T16:30:00Z""2025-04-21T16:30:00Z"
}
```

### Delete Event

**DELETE** `/events/{id}`

**Response**

```json
{
  "message": "Event deleted successfully"
}
```

### Filter Events by Tags (AND condition)

**GET** `/events?tags=user:rizqme,system:tester`

- Accepts a comma-separated list of tags
- Returns only events that match **all** provided tags

**Response**

```json
{
  "events": [
    {
      "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
      "title": "Team Sync",
      "tags": ["user:rizqme", "team:ai"],
      "start_time": "2025-05-01T09:00:00Z",
      "status": "active""active"
    }
  ]
}
```

### Filter Occurrences by Tags (AND condition)

**GET** `/occurrences?tags=user:rizqme,system:tester&page=1&limit=50`

- Filters occurrences whose parent event includes **all** specified tags.

**Response**

```json
{
  "occurrences": [
    {
      "id": "c1eaa746-b456-4e65-b239-51c045892ddb",
      "scheduled_at": "2025-05-01T09:00:00Z",
      "status": "scheduled",
      "last_attempt": null,
      "attempt_count": 0,
      "tags": ["user:rizqme", "team:ai"]
    },
    {
      "id": "c1eaa746-b456-4e65-b239-51c045892ddc",
      "scheduled_at": "2025-05-03T09:00:00Z",
      "status": "scheduled",
      "last_attempt": null,
      "attempt_count": 0
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 2
  }
}
```

### Get Occurrence

**GET** `/occurrences/{id}`

**Response**

```json
{
  "id": "c1eaa746-b456-4e65-b239-51c045892ddb",
  "event_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "scheduled_at": "2025-05-01T09:00:00Z",
  "status": "dispatched",
  "last_attempt": "2025-05-01T09:00:10Z",
  "attempt_count": 1,
  "tags": ["user:rizqme", "team:ai"]
}
```

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

