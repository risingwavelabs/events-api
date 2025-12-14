## Development

```shell
go install github.com/cloudcarver/anclax/cmd/anclax@latest
anclax gen # generate code after api.yaml/constructors/... are modified
make up
```

Open Console in http://localhost:8020, user/pass is root/root

Open the SQL editor and then create a table, says `CREATE TABLE test(c int)`

Insert with Event API:

```shell
curl -X POST -d '{"c": "1"}' -H 'Content-Type: application/json' 'http://localhost:8080/v1/events?name=test'
```

## Test

```shell
go test -count=1 -v -timeout 30s -run ^TestIngestEvents$ github.com/risingwavelabs/eventapi/tests
```

## Debug

Flame Graph

```shell
go tool pprof -http=:8779 http://127.0.0.1:8777/debug/pprof/profile\?seconds\=20 
```
