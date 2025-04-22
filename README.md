# Qhronos

Notification and Scheduling for AI Agents

## Quick Start Tutorial

This tutorial will guide you through setting up and running Qhronos on your local machine.

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

   redis:
     host: localhost
     port: 6379
     password: ""
     db: 0

   auth:
     master_token: "your-secure-master-token"
     jwt_secret: "your-secure-jwt-secret"
   ```

### Step 3: Start Required Services

1. Start PostgreSQL:
   ```bash
   ./scripts/db.sh start
   ```

2. Start Redis:
   ```bash
   ./scripts/redis.sh start
   ```

### Step 4: Install Dependencies

```bash
go mod download
```

### Step 5: Run the Server

```bash
go run cmd/server/main.go
```

The server should now be running at `http://localhost:8080`.

### Step 6: Create Your First Token

1. Use the master token to create a JWT token:
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

2. Save the returned JWT token for future requests.

### Step 7: Create Your First Event

1. Create an event using the JWT token:
   ```bash
   curl -X POST http://localhost:8080/events \
     -H "Authorization: Bearer your-jwt-token" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Daily Backup",
       "description": "System backup job",
       "schedule": "0 0 * * *",
       "tags": ["system:backup"],
       "metadata": {
         "backupType": "full",
         "retentionDays": 7
       }
     }'
   ```

### Step 8: Verify the Event

1. List all events:
   ```bash
   curl -X GET http://localhost:8080/events \
     -H "Authorization: Bearer your-jwt-token"
   ```

2. Get a specific event:
   ```bash
   curl -X GET http://localhost:8080/events/{event-id} \
     -H "Authorization: Bearer your-jwt-token"
   ```

### Step 9: Clean Up

When you're done, you can stop the services:

```bash
./scripts/db.sh stop
./scripts/redis.sh stop
```

## Common Issues and Solutions

### Database Connection Issues

If you encounter database connection issues:
1. Check if PostgreSQL is running: `./scripts/db.sh status`
2. Verify the connection details in `config.yaml`
3. Check if the port (5433) is available

### Redis Connection Issues

If you encounter Redis connection issues:
1. Check if Redis is running: `./scripts/redis.sh status`
2. Verify the connection details in `config.yaml`
3. Check if the port (6379) is available

### Authentication Issues

If you encounter authentication issues:
1. Verify your master token in `config.yaml`
2. Check if your JWT token is valid and not expired
3. Ensure you're using the correct Authorization header format

## Next Steps

- Explore the API documentation in the design document
- Set up monitoring and logging
- Configure rate limiting
- Implement webhook handlers for your events

## Setup

1. Install Go (1.24 or later)
2. Clone the repository
3. Copy `config.example.yaml` to `config.yaml` and update the configuration values
4. Install dependencies:
   ```bash
   go mod download
   ```
5. Start the required services (see Service Management section)
6. Run the server:
   ```bash
   go run cmd/server/main.go
   ```

## Service Management

The project uses Docker Compose to manage PostgreSQL and Redis services. Data is persisted in the `data` directory.

### Database Management

```bash
# Start the database
./scripts/db.sh start

# Stop the database
./scripts/db.sh stop

# Restart the database
./scripts/db.sh restart

# Remove the database container (data will be preserved)
./scripts/db.sh remove

# Check database status
./scripts/db.sh status
```

### Redis Management

```bash
# Start Redis
./scripts/redis.sh start

# Stop Redis
./scripts/redis.sh stop

# Restart Redis
./scripts/redis.sh restart

# Remove Redis container (data will be preserved)
./scripts/redis.sh remove

# Check Redis status
./scripts/redis.sh status

# Connect to Redis CLI
./scripts/redis.sh cli
```

### Service Configuration

#### Database
- Host: localhost
- Port: 5433
- User: postgres
- Password: postgres
- Database: qhronos

#### Redis
- Host: localhost
- Port: 6379
- Password: (none by default)
- DB: 0

These values match the default configuration in `config.yaml`.

## Configuration

The application uses a YAML configuration file (`config.yaml`) with the following structure:

```yaml
server:
  port: 8080

database:
  host: localhost
  port: 5433
  user: postgres
  password: postgres
  dbname: qhronos

redis:
  host: localhost
  port: 6379
  password: ""
  db: 0

auth:
  master_token: "your-master-token-here"
  jwt_secret: "your-jwt-secret-here"
```

### Authentication Configuration

- `master_token`: The master token used to create JWT tokens
- `jwt_secret`: The secret key used to sign JWT tokens

## Project Structure

```
qhronos/
├── cmd/
│   ├── api/            # API server entry point
│   └── server/         # Main application entry point
├── internal/
│   ├── api/            # API handlers and routes
│   ├── config/         # Configuration management
│   ├── models/         # Data models
│   ├── repository/     # Database operations
│   ├── scheduler/      # Scheduling logic
│   ├── services/       # Business logic
│   └── utils/          # Utility functions
├── pkg/
│   ├── auth/           # Authentication package
│   ├── database/       # Database package
│   └── redis/          # Redis package
├── data/
│   ├── postgres/       # PostgreSQL data directory
│   └── redis/         # Redis data directory
├── scripts/
│   ├── db.sh          # Database management script
│   ├── redis.sh       # Redis management script
│   └── test.sh        # Test runner script
├── migrations/        # Database migrations
└── tests/            # Test files
```

## Development

1. Make sure you have Docker installed and running
2. Start the required services using the management scripts
3. Update the configuration in `config.yaml`
4. Run the server:
   ```bash
   go run cmd/server/main.go
   ```

## Testing

The project uses a comprehensive test suite that includes:
- Unit tests for all components
- Integration tests for API endpoints
- Database tests with a dedicated test database

To run tests:
```bash
./scripts/test.sh
```

The test script will:
1. Start a test PostgreSQL container
2. Create and migrate the test database
3. Run all tests
4. Clean up the test environment

## Authentication

The API uses a two-tier authentication system:

1. **Master Token**
   - Used to create JWT tokens
   - Must be provided in the Authorization header
   - Format: `Bearer <master_token>`

2. **JWT Tokens**
   - Created using the master token
   - Include access level and scope information
   - Must be provided in the Authorization header
   - Format: `Bearer <jwt_token>`

### Access Levels
- `read`: Can only read resources
- `write`: Can read and write resources
- `admin`: Full access to all resources

### Scopes
- Define which resources a token can access
- Format: `resource:identifier`
- Example: `user:rizqme`, `system:tester`

## API Endpoints

### Authentication

#### Create JWT Token
```http
POST /tokens
Authorization: Bearer <master_token>

{
    "type": "jwt",
    "sub": "target_user_id",
    "access": "read",
    "scope": ["user:rizqme", "system:tester"],
    "expires_at": "2024-05-01T00:00:00Z"
}
```

Response:
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

### Events

#### Create Event
```http
POST /events
Authorization: Bearer <jwt_token>

{
    "name": "Test Event",
    "description": "Test Description",
    "schedule": "0 0 * * *",
    "tags": ["test"],
    "metadata": {
        "key": "value"
    }
}
```

Response:
```json
{
    "id": "72f358ee-a0b2-4ad4-a5bb-9061823d3480",
    "name": "Test Event",
    "description": "Test Description",
    "schedule": "0 0 * * *",
    "tags": ["test"],
    "metadata": {
        "key": "value"
    },
    "status": "active",
    "created_at": "2024-04-22T12:19:11Z",
    "updated_at": "2024-04-22T12:19:11Z"
}
```

#### Get Event
```http
GET /events/:id
Authorization: Bearer <jwt_token>
```

Response:
```json
{
    "id": "72f358ee-a0b2-4ad4-a5bb-9061823d3480",
    "name": "Test Event",
    "description": "Test Description",
    "schedule": "0 0 * * *",
    "tags": ["test"],
    "metadata": {
        "key": "value"
    },
    "status": "active",
    "created_at": "2024-04-22T12:19:11Z",
    "updated_at": "2024-04-22T12:19:11Z"
}
```

#### Update Event
```http
PUT /events/:id
Authorization: Bearer <jwt_token>

{
    "name": "Updated Event",
    "description": "Updated Description",
    "schedule": "0 0 * * *",
    "tags": ["test", "updated"],
    "metadata": {
        "key": "new_value"
    }
}
```

Response:
```json
{
    "id": "72f358ee-a0b2-4ad4-a5bb-9061823d3480",
    "name": "Updated Event",
    "description": "Updated Description",
    "schedule": "0 0 * * *",
    "tags": ["test", "updated"],
    "metadata": {
        "key": "new_value"
    },
    "status": "active",
    "created_at": "2024-04-22T12:19:11Z",
    "updated_at": "2024-04-22T12:19:11Z"
}
```

#### Delete Event
```http
DELETE /events/:id
Authorization: Bearer <jwt_token>
```

Response: 200 OK

#### List Events
```http
GET /events?tags=test,updated
Authorization: Bearer <jwt_token>
```

Response:
```json
[
    {
        "id": "72f358ee-a0b2-4ad4-a5bb-9061823d3480",
        "name": "Updated Event",
        "description": "Updated Description",
        "schedule": "0 0 * * *",
        "tags": ["test", "updated"],
        "metadata": {
            "key": "new_value"
        },
        "status": "active",
        "created_at": "2024-04-22T12:19:11Z",
        "updated_at": "2024-04-22T12:19:11Z"
    }
]
```

### Occurrences

#### Get Occurrence
```http
GET /occurrences/:id
Authorization: Bearer <jwt_token>
```

Response:
```json
{
    "id": "25c5d4ba-9c2b-4824-9a7f-bb46d148d11b",
    "event_id": "72f358ee-a0b2-4ad4-a5bb-9061823d3480",
    "scheduled_at": "2024-04-22T00:00:00Z",
    "status": "pending",
    "created_at": "2024-04-22T12:19:11Z",
    "updated_at": "2024-04-22T12:19:11Z"
}
```

#### List Occurrences
```http
GET /occurrences?tags=test
Authorization: Bearer <jwt_token>
```

Response:
```json
[
    {
        "id": "25c5d4ba-9c2b-4824-9a7f-bb46d148d11b",
        "event_id": "72f358ee-a0b2-4ad4-a5bb-9061823d3480",
        "scheduled_at": "2024-04-22T00:00:00Z",
        "status": "pending",
        "created_at": "2024-04-22T12:19:11Z",
        "updated_at": "2024-04-22T12:19:11Z"
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

## License

MIT
