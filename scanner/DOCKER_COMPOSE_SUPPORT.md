# Docker Compose Support in Fly.io

This document describes the Docker Compose scanner implementation for `flyctl launch` that enables deploying multi-container applications using Fly.io's multi-container machine support.

## Overview

The Docker Compose scanner detects `docker-compose.yml` or `docker-compose.yaml` files and translates them into Fly.io's multi-container machine configuration. This allows you to deploy existing Docker Compose applications to Fly.io with minimal changes.

## How It Works

1. **Detection**: The scanner looks for compose files in this order:
   - `docker-compose.yml`
   - `docker-compose.yaml`
   - `compose.yml`
   - `compose.yaml`

2. **Service Translation**: Each Docker Compose service is converted to a container in the machine configuration, except for database services which are recommended to be replaced with Fly.io managed services.

3. **Service Discovery Setup**: Automatically configures service discovery by:
   - Injecting an entrypoint script that updates `/etc/hosts`
   - Mapping all service names to `127.0.0.1` (localhost)
   - Chaining to the original entrypoint/command

4. **Configuration Generation**:
   - Creates a `fly.toml` file with basic app configuration
   - Generates a `fly.machine.json` file with multi-container specifications
   - Uses Pilot as the init system (required for multi-container machines)

## Supported Features

### ✅ Supported
- **Images**: Pre-built Docker images
- **Build contexts**: Local Dockerfile builds
- **Port mappings**: Translated to internal ports
- **Environment variables**: Passed through to containers
- **Dependencies**: `depends_on` relationships with conditions
- **Health checks**: Converted to Fly.io format
- **Volumes**: Translated to Fly.io persistent volumes
- **Restart policies**: Mapped to container restart settings
- **Service discovery**: Automatic `/etc/hosts` configuration for inter-service communication

### ⚠️ Partially Supported
- **Database services**: Detected but recommended to use managed services
- **Redis services**: Detected but recommended to use Upstash Redis
- **Networks**: Simplified since containers share VM networking

### ❌ Not Supported
- **External networks**: All containers run in the same VM
- **Privileged containers**: Security limitation
- **Host networking**: Not available in Fly.io

## Example Usage

Given a `docker-compose.yml` file:

```yaml
version: '3.8'
services:
  web:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgresql://user:pass@db:5432/myapp
    depends_on:
      - db
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  worker:
    build: .
    command: ["python", "worker.py"]
    environment:
      - DATABASE_URL=postgresql://user:pass@db:5432/myapp
    depends_on:
      - db

  db:
    image: postgres:13
    environment:
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=myapp
```

Running `flyctl launch` will:

1. **Detect** the Docker Compose configuration
2. **Generate** `fly.toml` with HTTP service configuration
3. **Create** `fly.machine.json` with multi-container setup
4. **Recommend** using Fly.io Postgres instead of the `db` service
5. **Configure** service discovery so containers can access each other by name
6. **Setup** containers to communicate via localhost (shared VM)

## Generated Configuration

### fly.toml
```toml
app = "my-app"
primary_region = "sea"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = true
  auto_start_machines = true
  min_machines_running = 0

machine_config = "fly.machine.json"
```

### fly.machine.json
```json
{
  "init": "pilot",
  "containers": [
    {
      "name": "web",
      "image": "registry.fly.io/my-app:web",
      "build": {
        "context": ".",
        "dockerfile": "Dockerfile"
      },
      "entrypoint": ["/fly-entrypoint.sh"],
      "environment": {
        "DATABASE_URL": "postgresql://user:pass@db:5432/myapp"
      },
      "depends_on": [
        {
          "container": "db",
          "condition": "started"
        }
      ],
      "healthcheck": {
        "test": ["CMD", "curl", "-f", "http://localhost:8080/health"],
        "interval": "30s",
        "timeout": "10s",
        "retries": 3
      }
    },
    {
      "name": "worker",
      "image": "registry.fly.io/my-app:worker",
      "build": {
        "context": ".",
        "dockerfile": "Dockerfile"
      },
      "entrypoint": ["/fly-entrypoint.sh"],
      "command": ["python", "worker.py"],
      "environment": {
        "DATABASE_URL": "postgresql://user:pass@db:5432/myapp"
      },
      "depends_on": [
        {
          "container": "db",
          "condition": "started"
        }
      ]
    }
  ]
}
```

## Database Recommendations

When the scanner detects database services, it will recommend using Fly.io managed services:

- **PostgreSQL** → `flyctl postgres create`
- **MySQL** → Fly.io MySQL (when available)
- **Redis** → `flyctl redis create` (Upstash Redis)

## Migration Tips

1. **Keep service names in connection strings**: The scanner automatically configures service discovery, so you can keep using service names like `db`, `redis`, etc.
2. **Remove database services**: Use managed services for better reliability
3. **Simplify networking**: Remove custom networks as containers communicate via localhost with service discovery
4. **Check resource limits**: Ensure containers fit within Fly.io machine limits

## Limitations

- All containers run in the same VM with shared resources
- No support for Docker Compose networks (containers use localhost)
- Database services should be replaced with managed services
- Some Docker Compose features may not translate directly

## Deployment

After `flyctl launch`, deploy with:

```bash
flyctl deploy
```

The deployment will:
1. Build images for containers with build contexts
2. Create multi-container machine with Pilot init
3. Start containers in dependency order
4. Configure health checks and networking

## Troubleshooting

**Build failures**: Ensure Dockerfiles are present in build contexts
**Networking issues**: Service names are automatically mapped to localhost via `/etc/hosts`
**Health check failures**: Verify health check commands work in containers
**Resource limits**: Check container resource requirements