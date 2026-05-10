# DataRouter Setup Guide for Linux

This guide documents every command needed to set up the DataRouter system from scratch on a Linux machine. It includes environment setup, dependencies, service build steps, container startup, and troubleshooting tips.

## 1. Install Linux Prerequisites

Open a terminal and run:

```bash
sudo apt update
sudo apt upgrade -y
sudo apt install -y git curl build-essential wget unzip
```

## 2. Install Go

Download and install a supported Go version:

```bash
cd /tmp
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
```

Add Go to your shell path:

```bash
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

Verify Go:

```bash
go version
```

## 3. Install Protocol Buffers Compiler and gRPC Plugins

```bash
sudo apt install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Ensure `$HOME/go/bin` is on your path:

```bash
echo $PATH | grep -q "$HOME/go/bin" || echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

Confirm installation:

```bash
protoc --version
which protoc-gen-go
which protoc-gen-go-grpc
```

## 4. Clone the DataRouter Repository

If you already have the repo locally, skip this step. Otherwise:

```bash
cd ~
git clone <your-repo-url> datarouter
cd datarouter
```

> In this workspace, the project path is `/home/xflow/datarouter`.

## 5. Install Docker and Docker Compose

Install Docker Engine:

```bash
sudo apt install -y ca-certificates curl gnupg lsb-release
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
```

Allow Docker without `sudo` (optional):

```bash
sudo usermod -aG docker $USER
newgrp docker
```

Check Docker:

```bash
docker version
docker compose version
```

## 6. Configure Database Containers

Create or update `docker-compose.yml` in `/home/xflow/datarouter` with these services:

```yaml
version: '3.9'
services:
  mongodb:
    image: mongo:latest
    container_name: datarouter-mongo
    ports:
      - "27017:27017"
    restart: unless-stopped

  redis:
    image: redis:latest
    container_name: datarouter-redis
    ports:
      - "6379:6379"
    restart: unless-stopped

  influxdb:
    image: influxdb:2.0
    container_name: datarouter-influxdb
    ports:
      - "8086:8086"
    environment:
      DOCKER_INFLUXDB_INIT_MODE: setup
      DOCKER_INFLUXDB_INIT_USERNAME: admin
      DOCKER_INFLUXDB_INIT_PASSWORD: password
      DOCKER_INFLUXDB_INIT_ORG: example-org
      DOCKER_INFLUXDB_INIT_BUCKET: datarouter
      DOCKER_INFLUXDB_INIT_TOKEN: my-token
    restart: unless-stopped
```

Start the containers:

```bash
cd /home/xflow/datarouter
docker compose up -d
```

Verify containers are running:

```bash
docker ps --filter "name=datarouter"
```

## 7. Install Go Dependencies

From the project root:

```bash
cd /home/xflow/datarouter
go mod tidy
```

This downloads:
- gRPC libraries
- PostgreSQL driver
- MongoDB driver
- Redis client
- InfluxDB client

## 8. Generate Protobuf Code

From project root:

```bash
protoc --go_out=. --go-grpc_out=. proto/storage.proto
```

This generates `proto/storage.pb.go` and `proto/storage_grpc.pb.go`.

## 9. Build All Service Binaries

Run from `/home/xflow/datarouter`:

```bash
go build -o bin/router cmd/router/main.go
go build -o bin/postgres cmd/postgres/main.go
go build -o bin/mongodb cmd/mongodb/main.go
go build -o bin/redis cmd/redis/main.go
go build -o bin/influxdb cmd/influxdb/main.go
```

If any binary fails, fix the Go compile errors first.

## 10. Start All Services

Run each service in its own terminal or background job:

```bash
cd /home/xflow/datarouter
./bin/postgres &
./bin/mongodb &
./bin/redis &
./bin/influxdb &
./bin/router &
```

Confirm the router listens on port 8080:

```bash
curl -I http://localhost:8080
```

## 11. Test the System

Store sample structured data:

```bash
curl -X POST http://localhost:8080/store \
  -H "Content-Type: application/json" \
  -d '{"name": "Ayesha", "email": "alice@test.com", "age": 25, "city": "LA"}'
```

Retrieve it from Postgres with the returned ID:

```bash
curl -X GET "http://localhost:8080/retrieve/postgres/<id>"
```

Store sample semi-structured data to MongoDB:

```bash
curl -X POST http://localhost:8080/store \
  -H "Content-Type: application/json" \
  -d '{"user": {"name": "Bob", "id": 123}, "tags": ["vip"], "active": true}'
```

## 12. Troubleshooting Notes

### A. `docker compose up -d` does not start a service
- Check the container logs:
  ```bash
  docker compose logs mongodb
  docker compose logs redis
  docker compose logs influxdb
  ```
- Verify `docker ps` lists the service.
- If a port is already used, stop the conflicting process or change the exposed port.

### B. `go build` fails due to missing packages
- Run:
  ```bash
  go mod tidy
  ```
- Confirm the import statements match the actual module names.
- If a package version is wrong, update `go.mod` and rerun `go mod tidy`.

### C. Router fails to connect to a handler
- Confirm the handler is running on the expected address and port.
- Check if the gRPC address in `cmd/router/main.go` matches the handler service addresses.
- Use `netstat -tlnp` or `ss -tlnp` to verify.

### D. InfluxDB returns unauthorized or 401 errors
- Confirm the init token and organization are configured in `docker-compose.yml`.
- Ensure the router uses the same token when connecting to InfluxDB.
- If the container was recreated, the init token may reset; restart the container.

### E. Redis connection refused
- Confirm the Redis container is running:
  ```bash
  docker ps | grep redis
  ```
- If Redis starts slowly, wait 5-10 seconds and retry.
- Ensure the router points at `localhost:6379` (or the service name in Docker Compose if using a Docker network).

### F. PostgreSQL table creation or query failures
- Verify the database connection string is correct.
- Confirm the target table exists and has the expected columns.
- If needed, open a shell and inspect manually:
  ```bash
  docker exec -it postgres psql -U postgres -d postgres -c "\dt"
  ```

### G. General debugging approach
- Check logs from the service binaries in the terminal where they run.
- Add temporary `fmt.Println` debug output in the Go service if the router classification or payload path is unclear.
- Verify the JSON payload is valid using `python -m json.tool` or an online validator.

## 13. Useful Commands Summary

```bash
# Start database containers
cd /home/xflow/datarouter
docker compose up -d

# Build binaries
go build -o bin/router cmd/router/main.go
go build -o bin/postgres cmd/postgres/main.go
go build -o bin/mongodb cmd/mongodb/main.go
go build -o bin/redis cmd/redis/main.go
go build -o bin/influxdb cmd/influxdb/main.go

# Start services
./bin/postgres &
./bin/mongodb &
./bin/redis &
./bin/influxdb &
./bin/router &

# Test API
curl -X POST http://localhost:8080/store -H "Content-Type: application/json" -d '{"name":"Ayesha","email":"alice@test.com","age":25,"city":"LA"}'
```

---

This file documents a complete from-scratch Linux setup for DataRouter, including the exact command sequence and the troubleshooting steps used to make the setup smooth and repeatable.