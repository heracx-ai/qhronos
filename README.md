# Qhronos

Notification and Scheduling for AI Agents

## Overview

Qhronos is a robust scheduling and notification service specifically designed for AI agents and automated systems. It provides a comprehensive API for managing events, schedules, and notifications with enterprise-grade features and reliability.

Key aspects of Qhronos include:

- **Advanced Scheduling**: Implements iCalendar recurrence rules (RFC 5545) for complex scheduling patterns, supporting:
  - Recurring events with flexible intervals
  - Day-of-week and day-of-month specifications
  - Timezone-aware scheduling
  - Exception dates and modifications

- **Webhook Integration**: Seamless integration with external systems through:
  - Configurable webhook endpoints
  - Automatic event trigger dispatching
  - Retry mechanisms for failed deliveries
  - Custom payload formatting

- **Security and Access Control**:
  - Two-tier authentication system (Master Token + JWT)
  - Role-based access control (RBAC)
  - Fine-grained permission management
  - Token-based API security

- **Data Management**:
  - PostgreSQL-backed durable storage
  - Event metadata support for custom data
  - Tag-based organization and filtering
  - Comprehensive event history tracking

- **Enterprise Features**:
  - High availability design
  - Scalable architecture
  - Comprehensive API documentation
  - Detailed error handling and logging
  - Rate limiting and request validation

Qhronos is particularly well-suited for:
- AI agent scheduling and coordination
- Automated system orchestration
- Enterprise workflow automation
- Distributed system event management
- Multi-tenant scheduling applications

### Core Capabilities

- **Advanced Scheduling**: Implements iCalendar (RFC 5545) recurrence rules for complex scheduling patterns, supporting:
  - Recurring events with flexible intervals
  - Day-of-week and day-of-month patterns
  - Timezone-aware scheduling
  - Exception dates and modifications

- **Webhook Integration**: Seamless integration with external systems through:
  - Configurable webhook endpoints
  - Automatic event trigger dispatching
  - Custom payload formatting
  - Retry mechanisms for failed deliveries

- **Security & Access Control**: Enterprise-grade security features:
  - Two-tier authentication system (Master Token + JWT)
  - Role-based access control (RBAC)
  - Fine-grained permission management
  - Token expiration and revocation

- **Data Management**: Robust data handling capabilities:
  - PostgreSQL-backed durable storage
  - Tag-based event organization
  - Custom metadata support
  - Event versioning and history

### Architecture

Qhronos is built with a microservices-ready architecture that emphasizes:
- **Scalability**: Horizontal scaling capabilities
- **Reliability**: Durable storage and transaction safety
- **Maintainability**: Clean separation of concerns
- **Extensibility**: Modular design for future enhancements

### Use Cases

Qhronos is ideal for:
- **AI Agent Scheduling**: Coordinate and schedule AI agent activities
- **Automated Workflows**: Trigger and manage automated processes
- **System Maintenance**: Schedule maintenance windows and updates
- **Data Processing**: Coordinate data processing pipelines
- **Monitoring Systems**: Schedule health checks and monitoring tasks

### Integration

The service is designed for easy integration with:
- AI agent frameworks
- Automation platforms
- Monitoring systems
- Custom applications
- Enterprise workflows

## Key Features

- **Flexible Scheduling**: Support for iCalendar recurrence rules (RFC 5545)
- **Webhook Integration**: Automatic webhook dispatching for event triggers
- **Role-Based Access Control**: Fine-grained access control with JWT tokens
- **Tag-Based Organization**: Organize events with custom tags
- **Metadata Support**: Store custom metadata with events
- **Durable Storage**: PostgreSQL-backed event storage
- **Enterprise Ready**: Built with security and scalability in mind

## Quick Start

### Prerequisites

1. Install Go (1.24 or later)
2. Install Docker and Docker Compose
3. Install Git

### Step 1: Clone the Repository

```bash
git clone https://github.com/feedloop/qhronos.git
cd qhronos
```

### Step 2: Set Up Configuration

1. Copy the example configuration file:
   ```bash
   cp config.example.yaml config.yaml
   ```

2. Edit `config.yaml` with your preferred settings:
   ```yaml
   server:
     port: 8080

   database:
     host: localhost
     port: 5433
     user: postgres
     password: postgres
     dbname: qhronos

   auth:
     master_token: "your-secure-master-token"
     jwt_secret: "your-secure-jwt-secret"
   ```

### Step 3: Start Required Services

1. Start PostgreSQL:
   ```bash
   ./scripts/db.sh start
   ```

2. Run database migrations:
   ```bash
   ./scripts/migrate.sh up
   ```

### Step 4: Install Dependencies

```bash
go mod download
```

### Step 5: Run the Server

```bash
go run cmd/api/main.go
```

The server should now be running at `http://localhost:8080`.

## Authentication

Qhronos uses a two-tier authentication system:

1. **Master Token**: A static token used to create JWT tokens
2. **JWT Tokens**: Short-lived tokens with specific access levels and scopes

### Creating a JWT Token

```bash
curl -X POST http://localhost:8080/tokens \
  -H "Authorization: Bearer your-secure-master-token" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "jwt",
    "sub": "your-user-id",
    "access": "admin",
    "scope": ["user:your-username"],
    "expires_at": "2024-12-31T23:59:59Z"
  }'
```

### Access Levels

- `read`: Can view events and occurrences
- `write`: Can create and update events
- `admin`: Full access to all operations

## API Endpoints

### Events

#### Create Event
```bash
curl -X POST http://localhost:8080/events \
  -H "Authorization: Bearer your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Daily Backup",
    "description": "System backup job",
    "start_time": "2024-03-20T00:00:00Z",
    "webhook_url": "https://example.com/webhook",
    "schedule": "FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1",
    "tags": ["system:backup"],
    "metadata": {
      "backupType": "full",
      "retentionDays": 7
    }
  }'
```

#### Get Event
```bash
curl -X GET http://localhost:8080/events/{event-id} \
  -H "Authorization: Bearer your-jwt-token"
```

#### Update Event
```bash
curl -X PUT http://localhost:8080/events/{event-id} \
  -H "Authorization: Bearer your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated Event Name",
    "description": "Updated description",
    "schedule": "FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1",
    "tags": ["system:backup", "priority:high"]
  }'
```

#### Delete Event
```bash
curl -X DELETE http://localhost:8080/events/{event-id} \
  -H "Authorization: Bearer your-jwt-token"
```

#### List Events by Tags
```bash
curl -X GET "http://localhost:8080/events?tags=system:backup" \
  -H "Authorization: Bearer your-jwt-token"
```

### Occurrences

#### Get Occurrence
```bash
curl -X GET http://localhost:8080/occurrences/{occurrence-id} \
  -H "Authorization: Bearer your-jwt-token"
```

#### List Occurrences by Event
```bash
curl -X GET "http://localhost:8080/events/{event-id}/occurrences" \
  -H "Authorization: Bearer your-jwt-token"
```

## Project Structure

```
qhronos/
├── cmd/                    # Main application entry points
│   └── api/               # API server entry point
├── internal/              # Private application code
│   ├── api/              # API definitions and interfaces
│   ├── config/           # Configuration management
│   ├── database/         # Database connection and setup
│   ├── handlers/         # HTTP request handlers
│   ├── middleware/       # HTTP middleware (auth, rate limit, etc.)
│   ├── models/           # Data models and structures
│   ├── repository/       # Data access layer
│   ├── services/         # Business logic services
│   │   └── scheduler/    # Scheduler and dispatcher services
│   ├── testutils/        # Testing utilities
│   └── utils/            # Shared utilities
├── migrations/           # Database migration scripts
├── pkg/                  # Public packages
├── scripts/             # Utility scripts
├── tests/               # Integration and end-to-end tests
├── data/                # Data files and fixtures
├── config.example.yaml  # Example configuration
├── design.md           # This design document
├── docker-compose.yml  # Docker development environment
├── go.mod              # Go module definition
├── go.sum              # Go module checksums
└── README.md           # Project documentation
```

## Error Handling

### Repository Layer

The repository layer follows a consistent pattern for handling not found cases:

1. **Get Operations**:
   - When a resource is not found, return `(nil, nil)`
   - This applies to methods like `GetByID`, `GetByName`, etc.

2. **Delete Operations**:
   - When deleting a non-existent resource, return `nil`
   - This applies to methods like `Delete`, `DeleteByID`, etc.

3. **Update Operations**:
   - When updating a non-existent resource, return `(nil, nil)`
   - This applies to methods like `Update`, `UpdateByID`, etc.

This pattern ensures consistent behavior across all repositories and simplifies error handling in the service layer.

## Status Endpoint

Qhronos provides a status endpoint for health checks and configuration overview:

```bash
curl -X GET http://localhost:8080/status
```

Response:
```json
{
  "status": "ok",
  "uptime_seconds": 86400,
  "version": "1.0.0",
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
  },
  "hmac": {
    "algorithm": "HMAC-SHA256",
    "signature_header": "X-Qhronos-Signature",
    "default_secret": "qhronos.io",
    "event_override_supported": true
  }
}
```

## Rate Limiting

Qhronos implements rate limiting using a token-bucket algorithm backed by Redis:

- **Bucket Size**: 100 requests
- **Refill Rate**: 10 requests per second
- **Window**: 1 second
- **Key Format**: `rate_limit:{token_id}`

Rate limit headers are included in responses:
- `X-RateLimit-Limit`: Maximum requests per window
- `X-RateLimit-Remaining`: Remaining requests in current window
- `X-RateLimit-Reset`: Time until rate limit resets

## License

MIT License - See LICENSE file for details
