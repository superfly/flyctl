# Docker Compose Scanner

The Docker Compose scanner enables `fly launch` to automatically detect and configure applications defined in `docker-compose.yml` files for deployment on Fly.io.

## Overview

The scanner automatically converts Docker Compose configurations to Fly.io multi-container deployments using the following approach:

- **Multi-container machines**: All services run in the same VM with shared networking
- **Service discovery**: Containers can communicate using localhost (127.0.0.1) and their respective port numbers
- **Database services**: Automatically detected and recommended to use Fly.io managed alternatives
- **Secrets management**: Docker Compose secrets are converted to Fly.io secrets as environment variables
- **Volume mapping**: Docker Compose volumes are converted to Fly.io mounts where applicable

## Supported Features

### Services
- **Image-based services**: Services using pre-built Docker images
- **Build-based services**: Services with Dockerfile builds (limited to one build service per compose file)
- **Port mapping**: Internal ports are extracted for HTTP service configuration
- **Environment variables**: Converted to container environment or Fly.io secrets for sensitive data
- **Health checks**: Docker Compose health checks are translated to Fly.io format
- **Dependencies**: Service dependencies are preserved (excluding database services)
- **Restart policies**: Basic restart policy translation

### Database Services
- **PostgreSQL**: Automatically detected and excluded from containers
- **MySQL**: Automatically detected and excluded from containers  
- **Redis**: Automatically detected and excluded from containers
- **Credential extraction**: Database environment variables are converted to Fly.io secrets

### Secrets
- **File-based secrets**: Secrets defined with `file:` are read and converted to environment variables
- **Service secret references**: Containers that reference secrets get access to them as environment variables
- **External secrets**: Skipped (must be managed separately)

### Volumes
- **Named volumes**: Converted to Fly.io volume mounts
- **External volumes**: Skipped

## File Detection

The scanner looks for these files in order of preference:
1. `docker-compose.yml`
2. `docker-compose.yaml`
3. `compose.yml`
4. `compose.yaml`

## Configuration Generation

### fly.toml
- Basic HTTP service configuration based on exposed ports
- Environment variables (non-sensitive)
- Volume mounts
- Container field (for single build service)
- Reference to machine configuration file

### fly.machine.json
- Multi-container configuration using Pilot init system
- Container definitions with image, environment, secrets, dependencies
- Service discovery setup (when needed)
- Health check translation
- File mounts (service discovery script)

### Service Discovery
For multi-container applications, a service discovery entrypoint script is generated that:
- Sets up `/etc/hosts` entries for inter-container communication
- Chains to original container entrypoints/commands
- Only applied when containers have explicit entrypoints/commands

## Limitations

### Single Build Service
Only one service in the Docker Compose file can have a `build` section. This service becomes the primary build target for the Fly.io application.

### Database Services
Database services (PostgreSQL, MySQL, Redis) are detected and excluded from the container configuration. The scanner recommends using Fly.io managed databases instead.

### Secrets as Files
Docker Compose secrets that are mounted as files are converted to environment variables in Fly.io, as Fly.io secrets work differently than Docker secrets.

### Networking
Docker Compose networking features like custom networks are not directly supported. All containers run in the same VM and can communicate via localhost.

## Example

Given this `docker-compose.yml`:

```yaml
version: "3.8"
services:
  web:
    build: .
    ports:
      - "80:3000"
    environment:
      - DATABASE_URL=postgres://user:pass@db/myapp
    secrets:
      - source: app_secret
        target: /app/secret.txt
    depends_on:
      - db
  
  db:
    image: postgres:15
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: user
      POSTGRES_PASSWORD: pass

secrets:
  app_secret:
    file: ./secret.txt
```

The scanner will:

1. **Detect database service**: `db` service identified as PostgreSQL and excluded
2. **Process web service**: Configured as main container with build context
3. **Extract secrets**: `app_secret` read from `./secret.txt` and created as Fly.io secret
4. **Configure networking**: Port 3000 exposed for HTTP service
5. **Create machine config**: Single container configuration with secret access
6. **Recommend managed DB**: Suggest using Fly.io PostgreSQL instead of container

Result:
- `fly.toml` with HTTP service on port 3000
- `fly.machine.json` with web container configuration
- `app_secret` available as environment variable to the container
- Recommendation to attach Fly.io PostgreSQL database

## Usage

```bash
# In directory with docker-compose.yml
fly launch

# The scanner automatically detects the Docker Compose configuration
# and guides you through the deployment process
```

## Best Practices

1. **Use managed databases**: Accept the scanner's recommendation to use Fly.io managed databases instead of running them in containers
2. **Single build service**: Keep only one service with a build section in your compose file
3. **Environment variables**: Use environment variables for secrets rather than mounted files when possible
4. **Health checks**: Define proper health checks in your compose file for better reliability
5. **Port configuration**: Ensure your application listens on `0.0.0.0` rather than `localhost` for proper connectivity

## Troubleshooting

### Container won't start
- Check if your application expects secrets as files vs environment variables
- Ensure your app listens on `0.0.0.0` instead of `localhost`
- Verify health check configuration

### Database connection issues
- Use Fly.io managed databases instead of container databases
- Update connection strings to use Fly.io database URLs
- Ensure database credentials are properly extracted as secrets

### Multiple build services error
- Only one service can have a `build` section
- Consider using pre-built images for additional services
- Or split into separate Fly.io applications

### Secrets not available
- Check that secrets are defined in the `secrets` section of compose file
- Verify the secret files exist and are readable
- Remember that Fly.io secrets are environment variables, not files