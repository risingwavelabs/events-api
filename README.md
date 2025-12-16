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

Ensure you have a running RisingWave instance. If you don't have one, you can start it with Docker:

```shell
docker run --rm -p 4566:4566 --name risingwave risingwavelabs/risingwave:v2.6.2
```

### Run Events API

#### Using Docker

```shell
docker run --rm \
  -e EAPI_RW_DSN=postgres://root:@host.docker.internal:4566/dev \
  -p 8000:8000 \
  --name events-api \
  risingwavelabs/events-api:latest
```

> **Note**: Use `host.docker.internal` to connect to RisingWave running on your host machine from within Docker.

#### Using Binary

Download and install the Events API binary:

```shell
curl -L https://rwtools.s3.amazonaws.com/eventapi/download.sh | sh
```

Run the Events API:

```shell
EAPI_RW_DSN='postgres://root:@localhost:4566/dev' ./eventapi
```

### Basic Usage

#### 1. Create a Table

First, create a table in RisingWave:

```shell
curl -X POST \
  -d 'CREATE TABLE test(id INT, name STRING)' \
  http://localhost:8000/v1/sql

# Wait briefly for table synchronization
sleep 1
```

#### 2. Insert Events

Use POST /v1/events to ingest data. It accepts a single JSON object or NDJSON (one JSON object per line). The endpoint returns 200 OK after the data is persisted in RisingWave. It performs automatic schema/type mapping and uses buffered batching to improve throughput, providing high-performance, at-least-once delivery over HTTP.

**Example: Insert JSON event**

```shell
curl -X POST \
  -d $'{"id": 1, "name": "Peter S"}' \
  'http://localhost:8000/v1/events?name=test'
```

**Example: Insert multiple NDJSON events**

```shell
curl -X POST \
  -d $'{"id": 2, "name": "Mike W"}\n{"id": 3, "name": "Mark S"}\n{"id": 4, "name": "Dylan G"}' \
  'http://localhost:8000/v1/events?name=test'
```

#### 3. Query Data

Query the ingested data:

```shell
curl -X POST \
  -d $'SELECT * FROM test' \
  http://localhost:8000/v1/sql
```

## Configuration

The Events API can be configured using environment variables or a YAML configuration file (`eventapi.yaml`). All environment variables use the `EAPI_` prefix.

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `EAPI_PORT` | HTTP server port | `8000` | No |
| `EAPI_HOST` | HTTP server host | `0.0.0.0` | No |
| `EAPI_RW_DSN` | RisingWave connection string | - | **Yes** |
| `EAPI_DEBUG_ENABLE` | Enable debug/profiling endpoints | `false` | No |
| `EAPI_DEBUG_PORT` | Debug server port | `8777` | No |

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
go test -count=1 -v -timeout 30s -run ^TestIngestEvents$ github.com/risingwavelabs/eventapi/tests
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
go build -o eventapi ./cmd/main.go

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o eventapi-linux-amd64 ./cmd/main.go

# Check version
./eventapi -version
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
EAPI_DEBUG_ENABLE=true EAPI_RW_DSN='postgres://root:@localhost:4566/dev' ./eventapi
```

Access pprof endpoints at `http://localhost:8777/debug/pprof/`

## License

Apache License 2.0
