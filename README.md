# RisingWave Events API

A lightweight HTTP API layer for ingesting data into RisingWave. The Events API provides a simple HTTP interface for real-time data ingestion and SQL query execution, making it easy to stream events into RisingWave without complex configuration.

## Features

- **Simple HTTP Ingestion**: Send JSON events directly to RisingWave tables via HTTP POST requests
- **SQL Execution**: Execute DDL and DML statements through an HTTP endpoint
- **Auto Schema Mapping**: Automatically maps JSON fields to table columns
- **Lightweight**: Minimal overhead between your application and RisingWave
- **Production Ready**: Built with Go for high performance and reliability

## Quick Start

### Prerequisites

Ensure you have a running RisingWave instance. If you don't have one, follow the [RisingWave Quick Start guide](https://docs.risingwave.com/get-started/quickstart) to get started.

### Run Events API

#### Using Docker

```shell
docker run --rm \
  -e EVENTS_API_RW_DSN=postgres://root:@localhost:4566/dev \
  -p 8000:8000 \
  --name events-api \
  risingwavelabs/events-api:v0.1.3
```

> **Note**: Replace the connection string with your own RisingWave DSN.

#### Using Binary

Download and install the Events API binary:

```shell
curl -L https://go.risingwave.com/events-api | sh
```

Run the Events API:

```shell
EVENTS_API_RW_DSN='postgres://root:@localhost:4566/dev' ./events-api
```

### Basic Usage

This example demonstrates how to ingest clickstream events from a web application into RisingWave for real-time analytics.

#### 1. Create a Clickstream Table

First, create a table to store clickstream events:

```shell
curl -X POST \
  -d 'CREATE TABLE clickstream (
    user_id BIGINT,
    session_id STRING,
    page_url STRING,
    event_type STRING,
    timestamp TIMESTAMP,
    referrer STRING,
    device_type STRING
  )' \
  http://localhost:8000/v1/sql
```

#### 2. Ingest Clickstream Events

Use POST /v1/events to ingest data. It accepts a single JSON object or NDJSON (one JSON object per line). The endpoint returns 200 OK after the data is persisted in RisingWave. It performs automatic schema/type mapping and uses buffered batching to improve throughput, providing high-performance, at-least-once delivery over HTTP.

**Example: Insert a single page view event**

```shell
curl -X POST \
  -d '{"user_id": 12345, "session_id": "sess_abc123", "page_url": "/products/laptop", "event_type": "page_view", "timestamp": "2024-01-15 10:30:00", "referrer": "https://google.com", "device_type": "desktop"}' \
  'http://localhost:8000/v1/events?name=clickstream'
```

**Example: Insert multiple events in NDJSON format**

You can send multiple events in a single request using NDJSON (newline-delimited JSON). Each line must be a valid JSON object:

```shell
curl -X POST \
  --data-binary @- \
  'http://localhost:8000/v1/events?name=clickstream' << 'EOF'
{"user_id": 12345, "session_id": "sess_abc123", "page_url": "/products/laptop", "event_type": "page_view", "timestamp": "2024-01-15 10:30:00", "referrer": "https://google.com", "device_type": "desktop"}
{"user_id": 12345, "session_id": "sess_abc123", "page_url": "/products/laptop", "event_type": "click", "timestamp": "2024-01-15 10:30:15", "referrer": "", "device_type": "desktop"}
{"user_id": 67890, "session_id": "sess_xyz789", "page_url": "/products/phone", "event_type": "page_view", "timestamp": "2024-01-15 10:31:00", "referrer": "https://twitter.com", "device_type": "mobile"}
EOF
```

#### 3. Query and Analyze Data

Query the ingested clickstream data:

```shell
# View all events
curl -X POST \
  -d 'SELECT * FROM clickstream ORDER BY timestamp DESC LIMIT 10' \
  http://localhost:8000/v1/sql

# Analyze page views by device type
curl -X POST \
  -d 'SELECT device_type, COUNT(*) as page_views 
      FROM clickstream 
      WHERE event_type = '\''page_view'\''
      GROUP BY device_type' \
  http://localhost:8000/v1/sql

# Find top pages
curl -X POST \
  -d 'SELECT page_url, COUNT(*) as views 
      FROM clickstream 
      WHERE event_type = '\''page_view'\''
      GROUP BY page_url 
      ORDER BY views DESC 
      LIMIT 5' \
  http://localhost:8000/v1/sql
```

## Configuration

The Events API can be configured using environment variables or a YAML configuration file (`events-api.yaml`). All environment variables use the `EVENTS_API_` prefix.

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `EVENTS_API_PORT` | HTTP server port | `8000` | No |
| `EVENTS_API_HOST` | HTTP server host | `0.0.0.0` | No |
| `EVENTS_API_RW_DSN` | RisingWave connection string | - | **Yes** |
| `EVENTS_API_DEBUG_ENABLE` | Enable debug/profiling endpoints | `false` | No |
| `EVENTS_API_DEBUG_PORT` | Debug server port | `8777` | No |

## Development

### Setting Up Development Environment

Start the development environment with Docker Compose:

```shell
make up
```

Start and watch the development container (hot reload):

```shell
make dev
```

Press `Ctrl + C` to stop the development container.

### Code Generation

The project uses [anclax](https://github.com/cloudcarver/anclax) for code generation. Install it and regenerate code after modifying API specifications:

```shell
# Install anclax
go install github.com/cloudcarver/anclax/cmd/anclax@latest

# Generate code after modifying api/v1.yaml or wire constructors
anclax gen
```

### Running Tests

End-to-end tests are available in `tests/e2e_test.go`:

```shell
# Run all tests
go test -v ./tests/...

# Run specific test
go test -count=1 -v -timeout 30s -run ^TestIngestEvents$ github.com/risingwavelabs/events-api/tests
```

### Debugging

The Events API includes pprof endpoints for profiling when debug mode is enabled.

Generate a flame graph:

```shell
go tool pprof -http=:8779 http://127.0.0.1:8777/debug/pprof/profile\?seconds\=20
```

## Building from Source

### Build the Binary

```shell
# Build for your current platform
go build -o events-api ./cmd/main.go

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o events-api-linux-amd64 ./cmd/main.go

# Check version
./events-api -version
```

### Build with Make

```shell
# Build all platform binaries
make build

# Binaries will be in ./bin/
ls -lh bin/
```

## Troubleshooting

### Table Not Found

**Problem**: Table doesn't exist when inserting events
```
Error: relation "table_name" does not exist
```

**Solutions**:
- Create the table first using `/v1/sql` endpoint
- Wait 1-2 seconds after creating a table before inserting events
- Verify table name matches exactly (case-sensitive)

### Debug Mode

Enable debug mode to access profiling endpoints:

```shell
EVENTS_API_DEBUG_ENABLE=true EVENTS_API_RW_DSN='postgres://root:@localhost:4566/dev' ./events-api
```

Access pprof endpoints at `http://localhost:8777/debug/pprof/`

## License

Apache License 2.0
