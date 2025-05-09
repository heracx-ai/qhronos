# Qhronos Design Document

## 1. Introduction

Qhronos is a developer-first, event scheduling and notification platform. It enables clients to schedule one-time or recurring events via a simple REST API, persist them reliably, and receive notifications through webhooks. Qhronos is designed for reliability, scalability, and extensibility, making it suitable for both cloud and on-premise deployments.

**Core Use Cases:**
- Scheduling time-based notifications or reminders
- Automating webhook calls for recurring business processes
- Building workflow triggers based on time or recurrence rules

**Value Proposition:**
- Durable, auditable event and occurrence tracking (PostgreSQL-backed)
- Simple, intuitive JSON APIs
- Secure, role-based access control and token management
- Reliable, retryable webhook delivery
- Modular, stateless, and scalable architecture

---

## 2. System Overview

### High-Level Architecture Diagram

```
[Client/API Consumer]
       │
       ▼
   [API Layer (Gin)]
       │
       ▼
   [Handlers]
       │
       ▼
   [Services] <─────────────┐
       │                    │
       ▼                    │
   [Repositories]           │
       │                    │
       ▼                    │
   [Database (Postgres)]    │
                            │
   [Middleware]─────────────┘
       │
       ▼
   [Scheduler Layer] ──► [Redis]
       │ (Core Scheduler, Expander, Dispatcher, Archival Scheduler)
       ▼
   [Background Jobs: Expander, Dispatcher, Archival/Cleanup]
       │
       ▼
   [Webhook Delivery]
```

### Component Summary Table

| Component         | Responsibility                                              |
|-------------------|------------------------------------------------------------|
| API Layer         | HTTP endpoints, routing, middleware wiring                  |
| Handlers          | Request validation, business logic orchestration            |
| Services          | Business logic, token/HMAC, scheduling logic                |
| Repositories      | Data access (sqlx), persistence                            |
| Database          | PostgreSQL, durable storage for events/occurrences         |
| Middleware        | Logging, error handling, authentication, rate limiting      |
| Scheduler Layer   | Expander, Dispatcher, Cleanup (background jobs)             |
| Redis             | Scheduling, rate limiting, background job coordination      |
| Webhook Delivery  | Reliable, retryable notification delivery                  |

---

## 3. Key Concepts

### Events
An **Event** is a user-defined action or reminder, scheduled to occur at a specific time or on a recurring basis. Each event contains metadata, a webhook URL for notification, and optional recurrence rules.

### Occurrences
An **Occurrence** represents a single scheduled execution of an event. For recurring events, multiple occurrences are generated according to the recurrence rules. Each occurrence tracks its status, attempts, and delivery history.

### Webhook Delivery
When an occurrence is due, Qhronos delivers a webhook (HTTP POST) to the event's configured URL. Delivery is signed (HMAC) for security and retried on failure according to policy.

### Scheduling & Recurrence
Qhronos supports both one-time and recurring events. Recurrence is defined using iCalendar (RFC 5545) rules. The scheduler expands recurring events into individual occurrences within a configurable lookahead window.

### Authentication & Authorization
Qhronos uses JWT tokens and a master token for API authentication. Access control is enforced via token scopes and roles, ensuring secure, multi-tenant operation.

---

## 4. Architecture Deep Dive

### API Layer
- Built with Gin (Go web framework)
- Exposes RESTful endpoints for all core resources
- Applies global and route-specific middleware

### Handlers
- Implement HTTP request handling for events, occurrences, tokens, and status
- Validate input, invoke business logic, and format responses

### Middleware
- Cross-cutting concerns: logging, error handling, authentication, rate limiting
- Ensures consistent request/response processing and security
- **Ginzap Logger:** Uses `zap` for structured logging of HTTP requests and responses, including latency, status, etc., via `ginzap.Ginzap` and `ginzap.RecoveryWithZap`.
- **RequestIDMiddleware:** Injects a unique request ID into the context and logs for improved traceability.

### Services
- Encapsulate business logic not directly tied to HTTP or data access
- Examples: token management, HMAC signing, scheduling logic

### Repository Layer
- Data access layer using sqlx for PostgreSQL
- Implements CRUD operations for events and occurrences
- Maps Go models to database tables

### Scheduler Layer
- Background jobs for event expansion, webhook dispatch, and data archival.
- Uses Redis for coordination and scheduling of tasks.
- **Core Scheduler (`scheduler.Scheduler`):** Manages the underlying scheduling mechanism in Redis, providing a foundation for other scheduler components like the Expander and Dispatcher to enqueue and manage scheduled tasks.
- **Expander (`scheduler.Expander`):** Periodically scans for recurring events. For each event, it computes the next set of occurrences based on the `look_ahead_duration` and `expansion_interval` settings and stores them (e.g., in Redis via the Core Scheduler, for the Dispatcher to pick up).
- **Dispatcher (`scheduler.Dispatcher`):** Retrieves due occurrences from the schedule (e.g., Redis, managed by Core Scheduler). It attempts to deliver the webhook to the event's configured URL, signing with HMAC if configured. Handles retries with backoff on failure, up to a maximum attempt count.
- **Archival Scheduler (`scheduler.ArchivalScheduler`):** Periodically archives old events, occurrences, and webhook attempts from the primary tables to archive tables based on configured retention policies and archival interval. This helps manage database size and performance.

### Models
- Defines data structures for events, occurrences, tokens, and errors
- Used throughout handlers, services, and repositories

### Database
- PostgreSQL is the source of truth for all persistent data
- Schema includes tables for events, occurrences, webhook attempts, analytics, and archives
- Indexes and triggers support performance and data integrity

### Configuration
- Loads settings from YAML files and environment variables
- Centralizes configuration for database, Redis, authentication, scheduler, and more

## 5. Business Logic Flows

### Event Lifecycle
- **Create:** Client sends a POST request to `/events` with event details. The handler validates input and stores the event in the database. If the event is recurring, it is marked for expansion.
- **Update:** Client sends a PUT request to `/events/{id}`. The handler validates and updates the event in the database. Changes to recurrence or schedule are reflected in future occurrences.
- **Delete:** Client sends a DELETE request to `/events/{id}`. The event and its future occurrences are removed or archived according to retention policy.

### Recurring Event Expansion
- The Expander job periodically scans for recurring events that need new occurrences generated (based on lookahead window).
- For each event, it computes the next set of occurrences and inserts them into the database.

### Occurrence State Transitions
- Occurrences are created with status `pending`.
- When scheduled for delivery, status changes to `scheduled`.
- On successful webhook delivery, status becomes `dispatched` or `completed`.
- On failure, status is set to `failed` after max retries.

### Webhook Dispatch & Retry
- The Dispatcher job finds due occurrences (status `scheduled` and time <= now).
- Attempts to deliver the webhook to the event's URL, signing with HMAC if configured.
- On success, updates status and logs the attempt.
- On failure, increments attempt count and retries with backoff, up to a maximum.

### Token Management
- Admins can create JWT tokens via the `/tokens` endpoint.
- Tokens encode access level and scope, controlling API permissions.

### Rate Limiting
- Each request is checked against a Redis-backed token bucket rate limiter.
- Exceeding the limit results in a 429 response.

### Archiving & Retention
- The **Archival Scheduler** job periodically archives old events, occurrences, and webhook attempts based on retention policies.
- Archived data is moved to separate tables for long-term storage.

### Health & Status Endpoints
- `/status` and `/health` endpoints provide service health and configuration information for monitoring and automation.

### Analytics & Metrics (Planned)
While the schema includes tables for analytics and performance metrics, the current implementation does not yet include background jobs or API endpoints for populating or exposing these metrics. This is a planned enhancement.

---

## 6. Data Model

### Entity-Relationship Diagram (Text-Based)

```
[events] <1-----n> [occurrences] <1-----n> [webhook_attempts]
   |                        |
   |                        +----< archived_occurrences >
   +----< archived_events >

[analytics_daily], [analytics_hourly], [performance_metrics] (aggregates)
```

### Table Descriptions

- **events:** Stores event definitions, metadata, schedule, and webhook configuration.
- **occurrences:** Each row represents a scheduled execution of an event, with status and delivery tracking.
- **webhook_attempts:** Logs each attempt to deliver a webhook for an occurrence, including status and response.
- **archived_events / archived_occurrences / archived_webhook_attempts:** Long-term storage for data past retention windows.
- **analytics_daily / analytics_hourly / performance_metrics:** Tables for aggregated statistics and system performance. **Note:** These tables are present in the schema for future use. As of the current implementation, they are not yet populated or queried by the application logic. Enhanced analytics and reporting are planned for a future release.

## 7. API Reference

### Endpoint Summary

| Method | Path                  | Description                        |
|--------|-----------------------|------------------------------------|
| POST   | /events               | Create a new event                 |
| GET    | /events/{id}          | Get event by ID                    |
| PUT    | /events/{id}          | Update event by ID                 |
| DELETE | /events/{id}          | Delete event by ID                 |
| GET    | /events               | List events (filterable by tags)   |
| GET    | /occurrences/{id}     | Get occurrence by ID               |
| GET    | /occurrences          | List occurrences (filterable)      |
| POST   | /tokens               | Create a new JWT token (admin)     |
| GET    | /status               | Service status and health info     |
| GET    | /health               | Simple health check                |
| WS     | /ws                   | Real-time event delivery via WebSocket |

### Request/Response Patterns
- All endpoints use JSON for request and response bodies.
- Standard HTTP status codes are used for success and error signaling.
- Error responses follow the format: `{ "error": "message" }`.

### Error Handling
- 400: Bad Request (invalid input)
- 401: Unauthorized (missing/invalid token)
- 403: Forbidden (insufficient permissions)
- 404: Not Found (resource does not exist)
- 429: Too Many Requests (rate limit exceeded)
- 500: Internal Server Error

**Note:** Analytics and metrics endpoints are not yet available. Planned for a future release.

## 8. Security

### Authentication
- **Master Token:** A static API key with full access, provided in the `Authorization: Bearer <master_token>` header.
- **JWT Token:** Issued via `/tokens` endpoint (admin only). Encodes user, access level, and scope. Provided in the `Authorization: Bearer <jwt_token>` header.

### Authorization
- Access is controlled by token type, access level (`read`, `write`, `admin`), and scope (tags).
- Master token bypasses all restrictions. JWT tokens are checked for required access and scope.

### Webhook HMAC Signing
- All webhook deliveries are signed using HMAC-SHA256.
- The signature is included in the `X-Qhronos-Signature` header.
- Each event can specify a custom secret, or use the system default.
- Recipients should verify the signature to ensure authenticity.

## 9. Operational Concerns

### Deployment
- Qhronos is designed for containerized deployment (Docker/Kubernetes).
- Requires PostgreSQL and Redis as external dependencies.
- Configuration is managed via YAML files and environment variables.

### Monitoring & Observability
- Exposes `/status` and `/health` endpoints for liveness and readiness checks.
- Metrics can be exported to Prometheus/Grafana for monitoring.
- Logs are structured and suitable for aggregation (e.g., ELK, Loki).

### Scaling
- Stateless API and background workers can be horizontally scaled.
- Database and Redis should be provisioned for high availability and throughput.

### Backup & Disaster Recovery
- Regular backups of PostgreSQL are recommended (e.g., pg_dump, managed backups).
- Redis persistence should be enabled for recovery of in-flight jobs.
- Disaster recovery procedures should include restoring both database and Redis state.

---

## 10. Extensibility & Future Enhancements

### Planned Features
- Webhook HMAC signing validation improvements
- Multi-tenant organization/user support
- Admin dashboard for event and system management
- Dead-letter queue management UI
- Rate-limited webhook batch dispatcher
- Enhanced analytics and reporting

### Extension Points
- New event types or scheduling strategies
- Custom authentication providers
- Additional notification channels (e.g., email, SMS)
- Pluggable storage or queue backends

### Planned Features
- Enhanced analytics and reporting (planned; schema support exists, but not yet implemented in code)

## 11. WebSocket Server Design

### Overview
Qhronos provides a WebSocket server to enable real-time event delivery for two types of clients:
- **Client-Hook Listener:** Listens for events where the event webhook is of the form `webhook: "q:<client-name>"`.
- **Tag-Based Listener:** Listens for events that have certain tags.

### Connection Types & Handshake
- Clients connect to the `/ws` endpoint and send an initial JSON message specifying:
  - `type`: `"client-hook"` or `"tag-listener"`
  - `client_name`: (for client-hook)
  - `tags`: (for tag-listener, array of strings)
  - `token`: (JWT or master token for authentication)
- The server authenticates the client and registers the connection under the appropriate registry.

### Message Flows
- **Event Dispatch:**
  - If an event's webhook is `q:<client-name>`, the event is sent to all `client-hook` connections for that client.
  - If an event has any of the subscribed tags, it is sent to all `tag-listener` connections for those tags.
- **Acknowledge (Ack):**
  - Only `client-hook` connections may send an `ack` message to confirm receipt of an event occurrence. The server marks the occurrence as acknowledged and may stop retries.
  - `tag-listener` connections receive an error if they send an `ack` message.

### Message Examples
- **Initial Handshake (client-hook):**
  ```json
  { "type": "client-hook", "client_name": "acme-corp", "token": "<JWT>" }
  ```
- **Initial Handshake (tag-listener):**
  ```json
  { "type": "tag-listener", "tags": ["billing", "urgent"], "token": "<JWT>" }
  ```
- **Event Message:**
  ```json
  { "type": "event", "event_id": "evt_123", "occurrence_id": "occ_456", "payload": { ... }, "tags": ["foo", "bar"] }
  ```
- **Acknowledge (client-hook only):**
  ```json
  { "type": "ack", "event_id": "evt_123", "occurrence_id": "occ_456" }
  ```
- **Error (e.g., ack from tag-listener):**
  ```json
  { "type": "error", "message": "acknowledge not supported for tag-listener" }
  ```

### Connection Lifecycle
- The server handles disconnects and cleans up registries.
- Only control frames (ping/pong/close) and, for client-hook, `ack` messages are allowed after handshake. Other messages result in an error.

### Security
- All connections are authenticated.
- Access control ensures clients only receive events for their own hooks or allowed tags.

---

## Event Model

Events now use an extensible action system for delivery. The `action` field specifies how the event is delivered.

### Event Table (simplified)
| Field         | Type                | Description                                 |
|-------------- |-------------------- |---------------------------------------------|
| id            | UUID                | Primary key                                 |
| name          | TEXT                | Event name                                  |
| description   | TEXT                | Event description                           |
| start_time    | TIMESTAMP           | When the event is scheduled to start        |
| action        | JSONB               | Action object (see below)                   |
| webhook       | TEXT                | (Deprecated, for backward compatibility)    |
| ...           | ...                 | ...                                         |

### Action Structure

The `action` field is a JSON object:
```json
{
  "type": "webhook" | "websocket" | "apicall",
  "params": { ... }
}
```
- **type**: The action type. Supported: `webhook`, `websocket`, `apicall`.
- **params**: Parameters for the action type.
  - For `webhook`: `{ "url": "https://..." }`
  - For `websocket`: `{ "client_name": "client1" }`
  - For `apicall`: `{ "method": "POST", "url": "https://...", "headers": { ... }, "body": "..." }`

#### Example: Webhook Action
```json
{
  "type": "webhook",
  "params": { "url": "https://example.com/webhook" }
}
```

#### Example: Websocket Action
```json
{
  "type": "websocket",
  "params": { "client_name": "client1" }
}
```

#### Example: API Call Action
```json
{
  "type": "apicall",
  "params": {
    "method": "POST",
    "url": "https://api.example.com/endpoint",
    "headers": { "Authorization": "Bearer token", "Content-Type": "application/json" },
    "body": "{ \"foo\": \"bar\" }"
  }
}
```

### Backward Compatibility
- The legacy `webhook` field is still supported for legacy clients. If provided, it is mapped to the appropriate `action`.

## Dispatcher and Action System

The dispatcher no longer switches directly on webhook/websocket. Instead, it delegates event delivery to the action system:
- The dispatcher uses an `ActionsManager` to execute the action specified in the event.
- Each action type (webhook, websocket, apicall) is registered with the manager and can be extended in the future.
- This design allows for easy addition of new action types (e.g., email, SMS, etc.) without changing the dispatcher logic.

