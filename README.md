# Qhronos

Notification and Scheduling for AI Agents

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

## Project Structure

```
qhronos/
├── cmd/
│   └── server/          # Main application entry point
├── internal/
│   ├── api/            # API handlers and routes
│   ├── config/         # Configuration management
│   ├── models/         # Data models
│   ├── repository/     # Database operations
│   ├── scheduler/      # Scheduling logic
│   ├── service/        # Business logic
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
│   └── redis.sh       # Redis management script
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

To run tests:
```bash
go test ./...
```

## License

MIT
