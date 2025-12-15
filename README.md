## Quick Start

Start RisingWave and eventapi in one TTY.
```shell
# download and install eventapi
curl -L https://rwtools.s3.amazonaws.com/eventapi/download.sh | sh

# start RisingWave
docker run --rm -p 24566:4566 -d --name rw-eventapi risingwavelabs/risingwave:v2.6.2

# start eventapi
EAPI_PORT=5070 EAPI_RW_DSN='postgres://root:@localhost:24566/dev' ./eventapi
```

Create table and insert data through eventapi.
```shell 
# Create table with psql (sleep 1 to wait for the synchronization)
psql "postgresql://root:@localhost:24566/dev" -c 'CREATE TABLE test(i INT, b BOOLEAN, s STRING, f FLOAT, j JSONB, a STRING[])' && sleep 1

# Insert data
curl -X POST -d '{"i": 1, "b": false, "s": "test", "f": 3.14, "j": {"nested": "value"}, "a": ["1", "2"]}' 'http://localhost:5070/v1/events?name=test'

# Check data
psql "postgresql://root:@localhost:24566/dev" -c 'SELECT * FROM test'
```

Clean up
```shell
docker stop rw-eventapi
```


## Development

Start development environment
```shell
make up

# Please Ctrl + C to stop only the dev container

# start and watch the dev container
make dev 
```

Install toolchain management tool anclax and generate code
```shell
go install github.com/cloudcarver/anclax/cmd/anclax@latest
anclax gen # generate code after api.yaml/constructors/... are modified
```

## Test

Check `tests/e2e_test.go` for more details.

```shell
go test -count=1 -v -timeout 30s -run ^TestIngestEvents$ github.com/risingwavelabs/eventapi/tests
```

## Debug

Flame Graph

```shell
go tool pprof -http=:8779 http://127.0.0.1:8777/debug/pprof/profile\?seconds\=20 
```
