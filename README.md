# RisingWave Events API

A lightweight HTTP API layer for ingesting data into RisingWave. The Events API provides a simple HTTP interface for real-time data ingestion and SQL query execution, making it easy to stream events into RisingWave without complex configuration.

## Features

- **Simple HTTP Ingestion**: Send JSON events directly to RisingWave tables via HTTP POST requests
- **SQL Execution**: Execute DDL and DML statements through a HTTP endpoint
- **Lightweight**: Minimal overhead between your application and RisingWave

## Quick Start

### Run Events API

#### Binary

Download and install the Events API binary:

```shell
curl -L https://rwtools.s3.amazonaws.com/eventapi/download.sh | sh
```

```shell
EAPI_RW_DSN='postgres://root:@localhost:4566/dev' ./eventapi
```

#### Docker

```shell
docker run -e EAPI_RW_DSN=postgres://root:@host.docker.internal:4566/dev --rm -p 8000:8000 --name events-api risingwavelabs/events-api:latest 
```

### Basic Usage

Create a table:

```shell
curl -X POST \
  -d 'CREATE TABLE test(i INT, b BOOLEAN, s STRING, f FLOAT, j JSONB, a STRING[])' \
  http://localhost:8000/v1/sql

# Wait briefly for table synchronization
sleep 1
```

Insert events:

```shell
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"i": 1, "b": false, "s": "test", "f": 3.14, "j": {"nested": "value"}, "a": ["1", "2"]}' \
  'http://localhost:8000/v1/events?name=test'
```

Query data:

```shell
curl -X POST \
  -d 'SELECT * FROM test' \
  http://localhost:8000/v1/sql
```

### Cleanup

```shell
docker stop rw-eventapi
```


## Configuration

The Events API can be configured using environment variables or a YAML configuration file (`eventapi.yaml`). All environment variables use the `EAPI_` prefix.

### Environment Variables

- `EAPI_PORT`: HTTP server port (default: 8000)
- `EAPI_HOST`: HTTP server host (default: 0.0.0.0)
- `EAPI_RW_DSN`: RisingWave connection string (required)
  - Format: `postgres://user:password@host:port/database`
  - Example: `postgres://root:@localhost:4566/dev`
- `EAPI_DEBUG_ENABLE`: Enable debug endpoints (default: false)
- `EAPI_DEBUG_PORT`: Debug server port (default: 8777)

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

```shell
# Build the binary
go build -o eventapi ./cmd/main.go

# Check version
./eventapi -version
```
