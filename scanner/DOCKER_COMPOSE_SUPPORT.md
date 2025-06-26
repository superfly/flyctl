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

2. **Service Translation**: Each Docker Compose service is converted to a container in the machine configuration, except for database services which are recommended to be replaced with Fly.io managed services. Dependencies on excluded database services are automatically removed.

3. **Service Discovery Setup**: Automatically configures service discovery by:
   - Injecting an entrypoint script (`/fly-entrypoint.sh`) that updates `/etc/hosts`
   - Mapping all service names to `127.0.0.1` (localhost)
   - Preserving original entrypoint/command behavior through chaining
   - Enabling containers to access each other using their Docker Compose service names

4. **Configuration Generation**:
   - Creates a `fly.toml` file with basic app configuration
   - Generates a `fly.machine.json` file with multi-container specifications
   - Configures for multi-container deployment (Pilot init is used automatically)
   - Includes the service discovery entrypoint script

## Supported Features

### ✅ Supported
- **Images**: Pre-built Docker images
- **Build contexts**: Local Dockerfile builds
- **Port mappings**: Translated to internal ports
- **Environment variables**: Passed through to containers
- **Dependencies**: `depends_on` relationships with conditions (excluding database services)
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

## Service Discovery

The Docker Compose scanner automatically sets up service discovery to maintain compatibility with existing Docker Compose applications:

### How Service Discovery Works

1. **Entrypoint Script**: Each container gets an entrypoint script (`/fly-entrypoint.sh`) that:
   - Appends service names to `/etc/hosts` pointing to `127.0.0.1`
   - Preserves existing Fly.io networking entries (6PN addresses, fly-local-6pn, etc.)
   - Only adds entries if they don't already exist
   - Chains to the original entrypoint or command
   - Maintains all original container behavior

2. **Name Resolution**: Containers can access each other using service names:
   - `http://web:3000` → resolves to `http://127.0.0.1:3000`
   - `redis://cache:6379` → resolves to `redis://127.0.0.1:6379`
   - `postgresql://db:5432/myapp` → resolves to `postgresql://127.0.0.1:5432/myapp`

3. **Automatic Configuration**: No changes needed to:
   - Connection strings
   - Environment variables
   - Application code

### Example Service Discovery

If your `docker-compose.yml` defines services named `web`, `api`, and `cache`, the entrypoint script will append to `/etc/hosts`:

```bash
# Original Fly.io entries preserved
127.0.0.1    localhost
fdaa:0:cfd4:a7b:c988:593f:a9e5:2    fly-local-6pn
172.19.10.34    178190eb632e58
172.19.10.35    fly-global-services
2605:4c40:119:c6b2:0:593f:a9e5:1    178190eb632e58

# Service discovery entries added by fly-entrypoint.sh
127.0.0.1    web
127.0.0.1    api
127.0.0.1    cache
```

This allows your `web` container to access the `api` container using `http://api:8080` just like in Docker Compose, while maintaining all Fly.io networking functionality.

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
      - REDIS_URL=redis://cache:6379
    depends_on:
      - db
      - cache
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
      - REDIS_URL=redis://cache:6379
    depends_on:
      - db
      - cache

  cache:
    image: redis:alpine

  db:
    image: postgres:13
    environment:
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=myapp
```

Running `flyctl launch` will:

1. **Detect** the Docker Compose configuration
2. **Generate** `fly.toml` with HTTP service configuration
3. **Create** `fly-entrypoint.sh` script for service discovery
4. **Generate** `fly.machine.json` with multi-container setup
5. **Recommend** using Fly.io Postgres and Redis instead of containerized versions
6. **Configure** service discovery so containers can access each other by name
7. **Setup** containers to communicate via localhost (shared VM)

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

### fly-entrypoint.sh
```bash
#!/bin/sh
set -e

# Add service names to /etc/hosts for multi-container service discovery
# This allows containers to access each other using their service names
# We append to the existing /etc/hosts to preserve Fly.io networking entries

# Only add entries if they don't already exist
if ! grep -q "\sweb\(\s\|$\)" /etc/hosts; then
    echo "127.0.0.1    web" >> /etc/hosts
fi
if ! grep -q "\sworker\(\s\|$\)" /etc/hosts; then
    echo "127.0.0.1    worker" >> /etc/hosts
fi

# Chain to the original entrypoint or command
if [ $# -eq 0 ]; then
    # No arguments provided, this shouldn't happen in multi-container setup
    exec /bin/sh
elif [ -x "$1" ]; then
    # First argument is executable, run it directly
    exec "$@"
else
    # First argument is not executable, run it with shell
    exec /bin/sh -c "$*"
fi
```

### fly.machine.json
```json
{
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
        "DATABASE_URL": "postgresql://user:pass@db:5432/myapp",
        "REDIS_URL": "redis://cache:6379"
      },
      "files": [
        {
          "guest_path": "/fly-entrypoint.sh",
          "local_path": "fly-entrypoint.sh"
        }
      ],
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
        "DATABASE_URL": "postgresql://user:pass@db:5432/myapp",
        "REDIS_URL": "redis://cache:6379"
      },
      "files": [
        {
          "guest_path": "/fly-entrypoint.sh",
          "local_path": "fly-entrypoint.sh"
        }
      ],
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

### Why Use Managed Services?

1. **Better Performance**: Dedicated resources and optimized configurations
2. **Automatic Backups**: Built-in backup and restore capabilities
3. **High Availability**: Replication and failover support
4. **Security**: Network isolation and encryption at rest
5. **Monitoring**: Built-in metrics and alerting

## Migration Tips

1. **Keep service names in connection strings**: The scanner automatically configures service discovery, so you can keep using service names like `db`, `redis`, `cache`, etc.
2. **Remove database services**: Use managed services for better reliability and performance
3. **Simplify networking**: Remove custom networks as containers communicate via localhost with service discovery
4. **Check resource limits**: Ensure containers fit within Fly.io machine limits
5. **Test locally first**: Verify your application works with all services on localhost
6. **Use environment variables**: Keep connection strings in environment variables for easy updates

## Limitations

- All containers run in the same VM with shared resources
- No support for Docker Compose networks (containers use localhost)
- Database services should be replaced with managed services
- Some Docker Compose features may not translate directly
- Maximum number of containers limited by VM resources
- No support for container-to-container volumes

## Deployment

After `flyctl launch`, deploy with:

```bash
flyctl deploy
```

The deployment will:
1. Build images for containers with build contexts
2. Upload images to Fly.io registry
3. Create multi-container machine (with automatic Pilot init)
4. Start containers in dependency order
5. Configure health checks and networking
6. Set up service discovery via `/etc/hosts`

## Troubleshooting

### Common Issues

**Build failures**:
- Ensure Dockerfiles are present in build contexts
- Check that build contexts are relative to docker-compose.yml location
- Verify Docker images are accessible if using pre-built images

**Networking issues**:
- Service names are automatically mapped to localhost via `/etc/hosts`
- Ensure services listen on the correct ports
- Check that services bind to `0.0.0.0` not just `127.0.0.1`

**Health check failures**:
- Verify health check commands work in containers
- Ensure health check endpoints are accessible
- Check timeout values are appropriate for your application

**Resource limits**:
- Check container resource requirements
- Monitor VM CPU and memory usage
- Consider scaling horizontally with multiple machines

**Service discovery problems**:
- Verify the entrypoint script is being executed
- Check container logs for `/etc/hosts` setup errors
- Ensure original entrypoint/command is preserved

### Debugging Commands

```bash
# View machine configuration
flyctl machine list

# Check container logs
flyctl logs

# SSH into machine
flyctl ssh console

# Inspect /etc/hosts in a container
flyctl ssh console -C "docker exec <container-name> cat /etc/hosts"

# View machine status
flyctl status
```

## Advanced Configuration

### Custom Entrypoint Handling

If your container has a complex entrypoint, the scanner will:
1. Set `/fly-entrypoint.sh` as the new entrypoint
2. Pass the original entrypoint as the command
3. Chain execution to preserve behavior

### Environment Variable Substitution

The scanner preserves Docker Compose environment variables, but you may need to update:
- Database connection strings to use Fly.io managed services
- External service URLs to use Fly.io equivalents
- File paths to match Fly.io volume mounts

### Volume Migration

Docker Compose volumes are converted to Fly.io persistent volumes:
- Named volumes → Fly.io volumes with same name
- Bind mounts → Not supported, use Fly.io volumes instead
- Anonymous volumes → Create named Fly.io volumes

## Best Practices

1. **Start Simple**: Test with a minimal configuration first
2. **Use Health Checks**: Ensure all services have proper health checks
3. **Monitor Resources**: Use `flyctl status` and metrics to track usage
4. **Plan for Failures**: Implement proper error handling and retries
5. **Document Changes**: Keep notes on any modifications from Docker Compose
6. **Test Thoroughly**: Verify all service interactions work correctly
