# db-as-a-service

A REST API server that exposes [micro-cassandra](../micro-cassandra) as a multi-tenant key-value store. Each tenant gets an isolated named database backed by its own LSM-tree storage directory.

## Goal

Build a working database-as-a-service in under one hour (including tests), using only the Go standard library and the micro-cassandra storage engine.

## API

All request and response bodies are JSON. Keys are passed as URL path segments.

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Returns `{"status": "ok"}` |

### Databases

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/databases/{name}` | Create a new database |
| `GET` | `/databases` | List all databases |
| `GET` | `/databases/{name}/stats` | Database stats (key count estimate, SSTable count) |
| `DELETE` | `/databases/{name}` | Delete a database and its data |

### Keys

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/databases/{name}/keys/{key}` | Write a value |
| `GET` | `/databases/{name}/keys/{key}` | Read a value |

### Request/response examples

**Create a database:**

```
POST /databases/mydb
-> 201 {"name": "mydb"}
```

**Write a key:**

```
PUT /databases/mydb/keys/greeting
Content-Type: application/json

{"value": "hello world"}
-> 200 {"key": "greeting", "value": "hello world"}
```

**Read a key:**

```
GET /databases/mydb/keys/greeting
-> 200 {"key": "greeting", "value": "hello world"}
-> 404 {"error": "key not found"}
```

**List databases:**

```
GET /databases
-> 200 {"databases": ["mydb", "other"]}
```

**Database stats:**

```
GET /databases/mydb/stats
-> 200 {"name": "mydb", "sstable_count": 3}
```

**Delete a database:**

```
DELETE /databases/mydb
-> 200 {"name": "mydb"}
```

## Auto-compaction

A background goroutine runs periodically (every 30 seconds by default) and calls `Compact()` on each open database. This keeps SSTable count low without blocking writes.

## What is intentionally omitted

- Authentication / authorization
- Key listing or range scans (micro-cassandra only supports point lookups)
- Custom per-database configuration
- Replication
- Rate limiting

## Project structure

```
main.go          -- entry point, flag parsing, server startup, graceful shutdown
server.go        -- HTTP handler routing and request/response helpers
dbmanager.go     -- manages named DB instances (create, get, delete, list)
compactor.go     -- background auto-compaction loop
server_test.go   -- integration tests exercising the full API
```

## Implementation plan

1. **Skeleton** -- `go.mod` with local replace for micro-cassandra, `main.go` that starts an HTTP server with `/health`, graceful shutdown on SIGINT/SIGTERM.
2. **DB manager** -- `dbmanager.go`: a concurrent-safe registry that creates, retrieves, lists, and deletes named `microcassandra.DB` instances. Each database gets its own subdirectory under a root data dir.
3. **Key-value endpoints** -- `server.go`: `PUT /databases/{name}/keys/{key}` and `GET /databases/{name}/keys/{key}`, wired to the DB manager.
4. **Database management endpoints** -- `POST /databases/{name}`, `GET /databases`, `GET /databases/{name}/stats`, `DELETE /databases/{name}`.
5. **Auto-compaction** -- `compactor.go`: background goroutine that periodically compacts all open databases, cancelled via context on shutdown.
6. **Integration tests** -- `server_test.go`: start a real server, exercise the golden path and error cases for every endpoint.

## Running

```sh
go run . -addr :8080 -data ./data
```

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:8080` | Listen address |
| `-data` | `./data` | Root directory for database storage |

## Testing from the CLI

Start the server in one terminal, then run these commands in another.

**Health check:**
```sh
curl -s http://localhost:8080/health
# -> {"status":"ok"}
```

**Create a database:**
```sh
curl -s -X POST http://localhost:8080/databases/mydb
# -> {"name":"mydb"}
```

**Write a key:**
```sh
curl -s -X PUT http://localhost:8080/databases/mydb/keys/greeting \
  -H 'Content-Type: application/json' \
  -d '{"value": "hello world"}'
# -> {"key":"greeting","value":"hello world"}
```

**Read a key:**
```sh
curl -s http://localhost:8080/databases/mydb/keys/greeting
# -> {"key":"greeting","value":"hello world"}
```

**Read a missing key:**
```sh
curl -s http://localhost:8080/databases/mydb/keys/nope
# -> {"error":"key not found"}
```

**List databases:**
```sh
curl -s http://localhost:8080/databases
# -> {"databases":["mydb"]}
```

**Database stats:**
```sh
curl -s http://localhost:8080/databases/mydb/stats
# -> {"name":"mydb","sstable_count":0}
```

**Delete a database:**
```sh
curl -s -X DELETE http://localhost:8080/databases/mydb
# -> {"name":"mydb"}
```
