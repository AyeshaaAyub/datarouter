# DataRouter Interaction Scenarios

This document provides multiple scenarios for interacting with the DataRouter system, demonstrating storage, retrieval, and error handling across different data types and databases.

## Prerequisites

Ensure services are running:
```bash
cd /home/xflow/datarouter

# Start database containers
docker compose up -d

# Start handlers and router
./bin/postgres &
./bin/mongodb &
./bin/redis &
./bin/influxdb &
./bin/router &
```

## Scenario 1: Store and Retrieve Structured Data (Postgres)

```bash
# Store structured data (4+ fields → Postgres)
curl -X POST http://localhost:8080/store \
  -H "Content-Type: application/json" \
  -d '{"name": "Alice", "email": "alice@example.com", "age": 30, "city": "NYC"}'
# Response: {"db_type":"postgres","id":"4"}

# Retrieve it
curl -X GET "http://localhost:8080/retrieve/postgres/4"
# Response: {"data":{"age":30,"city":"NYC","email":"alice@example.com","name":"Alice"},"db_type":"postgres","id":"4"}
```

## Scenario 2: Store Semi-Structured Data (MongoDB)

```bash
# Store flexible data (→ MongoDB)
curl -X POST http://localhost:8080/store \
  -H "Content-Type: application/json" \
  -d '{"user": {"name": "Bob", "id": 123}, "tags": ["vip", "premium"], "active": true}'
# Response: {"db_type":"mongodb","id":"ObjectID(\"6a0056711c8daae8c837f695\")"}

# Retrieve it
curl -X GET "http://localhost:8080/retrieve/mongodb/6a0056711c8daae8c837f695"
# Response: {"data":{"_id":"ObjectID(\"6a0056711c8daae8c837f695\")","active":true,"tags":["vip","premium"],"user":{"id":123,"name":"Bob"}},"db_type":"mongodb","id":"6a0056711c8daae8c837f695"}
```

## Scenario 3: Use Collections

```bash
# Store in specific collection
curl -X POST "http://localhost:8080/store?collection=users" \
  -H "Content-Type: application/json" \
  -d '{"name": "Charlie", "email": "charlie@test.com", "age": 25}'
# Response: {"db_type":"mongodb","id":"ObjectID(\"6a0056ad1c8daae8c837f696\")"}

# Retrieve from collection
curl -X GET "http://localhost:8080/retrieve/mongodb/6a0056ad1c8daae8c837f696?collection=users"
# Response: {"data":{"_id":"ObjectID(\"6a0056ad1c8daae8c837f696\")","age":25,"email":"charlie@test.com","name":"Charlie"},"db_type":"mongodb","id":"6a0056ad1c8daae8c837f696"}
```

## Scenario 4: Error Handling (Not Found)

```bash
# Try to retrieve non-existent record
curl -X GET "http://localhost:8080/retrieve/postgres/999"
# Response: record not found (HTTP 404)
```

## Scenario 5: Time-Series Data (InfluxDB - requires container)

```bash
# Start InfluxDB container first
docker compose up -d influxdb

# Then store time-series data
curl -X POST http://localhost:8080/store \
  -H "Content-Type: application/json" \
  -d '{"timestamp": "2026-05-10T10:00:00Z", "sensor": "temp-01", "value": 23.5}'
# Response: {"db_type":"influxdb","id":"influx-..."}
```

## Scenario 6: Key-Value Data (Redis - requires container)

```bash
# Start Redis container first
docker compose up -d redis

# Then store simple key-value
curl -X POST http://localhost:8080/store \
  -H "Content-Type: application/json" \
  -d '{"user_id": "12345", "status": "active"}'
# Response: {"db_type":"redis","id":"default:redis-..."}
```

## Scenario 7: Alternative Retrieve Syntax

```bash
# Use query parameters instead of path
curl -X GET "http://localhost:8080/retrieve?id=4&db_type=postgres&collection=default"
# Same response as path-based retrieval
```

## Scenario 8: Invalid Requests

```bash
# Empty payload
curl -X POST http://localhost:8080/store -H "Content-Type: application/json" -d '{}'
# Response: empty payload (HTTP 400)

# Wrong method
curl -X PUT http://localhost:8080/store -d '{"test": "data"}'
# Response: method not allowed (HTTP 405)
```

## Scenario 9: Check Database Contents Directly

```bash
# Postgres data
docker exec postgres psql -U postgres -d postgres -c "SELECT * FROM data_router;"

# MongoDB data
docker exec datarouter-mongo mongosh --eval "db.datarouter.find().toArray()"
```

## Scenario 10: Stop and Restart Services

```bash
# Kill all background processes
pkill -f "./bin/"

# Restart services
./bin/postgres &
./bin/mongodb &
./bin/redis &
./bin/influxdb &
./bin/router &
```

## Data Classification Rules

| Pattern | Detected By | Routed To | Example |
|---------|------------|-----------|---------|
| Has `timestamp` field | String/Time parsing | InfluxDB | `{"timestamp": "2026-05-08T...", "value": 42}` |
| ≤2 primitive fields | Type checking | Redis | `{"key": "val", "status": "ok"}` |
| ≥4 fields | Field count | Postgres | `{"name": "x", "email": "y", "age": 30, "city": "z"}` |
| Other | Default | MongoDB | Anything not matching above |

## API Endpoints

- **POST /store** - Store data (auto-classifies and routes)
- **GET /retrieve/{db_type}/{id}** - Retrieve by ID and type
- **GET /retrieve?id=...&db_type=...&collection=...** - Alternative retrieve syntax

All services are now running and tested. Use `docker compose up -d` to start additional databases as needed. The router automatically classifies and routes data based on structure.