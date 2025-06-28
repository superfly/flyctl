# Announcing Docker Compose Support for Fly.io: Deploy Multi-Container Apps with Zero Configuration Changes

Today, we're excited to announce native Docker Compose support in `flyctl launch`, making it easier than ever to deploy your existing multi-container applications to Fly.io. With this new feature, you can take your Docker Compose applications and deploy them to Fly.io's global infrastructure without modifying a single line of code.

## The Challenge: Multi-Container Deployment Complexity

Many modern applications use Docker Compose to orchestrate multiple services—a web server, background workers, databases, and caches all working together. Until now, deploying these applications to cloud platforms often meant:

- Rewriting configuration files for each platform
- Manually setting up service discovery between containers
- Recreating complex networking and dependency relationships
- Managing secrets and environment variables across different systems

This friction has been a significant barrier to adoption, especially for teams with existing Docker Compose workflows who want the benefits of edge deployment without the migration headache.

## Introducing Zero-Friction Docker Compose Deployment

Our new Docker Compose scanner automatically detects `docker-compose.yml` files and translates them into Fly.io's multi-container machine configuration. Here's what happens when you run `flyctl launch` in a directory with a Docker Compose file:

```bash
$ flyctl launch
Scanning source code
Detected a Docker Compose 3.8 app
✓ Detected multi-container application with 3 services
  Created entrypoint script: fly-entrypoint.sh
  Created machine configuration file: fly.machine.json
  Note: All containers will run in the same VM with shared networking
```

### Automatic Service Discovery

One of the most powerful features is automatic service discovery. Your containers can communicate with each other using the same service names they used locally:

```yaml
# docker-compose.yml
services:
  web:
    image: nginx:latest
    depends_on: [api, cache]
  api:
    image: myapp:latest
    environment:
      - REDIS_URL=redis://cache:6379
      - DATABASE_URL=postgresql://db:5432/myapp
  cache:
    image: redis:alpine
```

The scanner automatically injects an entrypoint script that updates `/etc/hosts`, mapping service names to `127.0.0.1`. Your `api` container can still connect to `redis://cache:6379` without any code changes—it just works. This means you can test your complete application locally with `docker-compose up` and deploy the exact same configuration to production with confidence.

### Smart Database Integration

When the scanner detects database services, it recommends Fly.io's managed alternatives:

```bash
! Database service detected in docker-compose.yml
  Consider using Fly.io managed databases for better performance and reliability
```

Database credentials are automatically extracted from environment variables and converted to Fly.io secrets, while managed database connections are handled seamlessly. This gives you production-ready database infrastructure without the operational overhead.

### Clean Configuration Generation

The scanner generates clean, minimal configuration files. For Docker Compose files using only external images, no unnecessary build sections are created:

```toml
# fly.toml - Clean and minimal
app = "my-app"
primary_region = "sea"
machine_config = "fly.machine.json"

[http_service]
  internal_port = 80
  force_https = true
```

## Real-World Example: From Docker Compose to Global Edge

Let's walk through deploying a typical web application with a database and cache:

**Before** - Your existing `docker-compose.yml`:
```yaml
version: '3.8'
services:
  web:
    build: .
    ports: ["8080:8080"]
    environment:
      - DATABASE_URL=postgresql://user:pass@db:5432/myapp
      - REDIS_URL=redis://cache:6379
    depends_on: [db, cache]
    volumes:
      - "./nginx.conf:/etc/nginx/conf.d/default.conf:ro"

  worker:
    build: .
    command: ["python", "worker.py"]
    environment:
      - DATABASE_URL=postgresql://user:pass@db:5432/myapp
      - REDIS_URL=redis://cache:6379
    depends_on: [db, cache]

  cache:
    image: redis:alpine

  db:
    image: postgres:13
    environment:
      - POSTGRES_PASSWORD=secret
```

**After** - Just run `flyctl launch`:
```bash
$ flyctl launch
Detected a Docker Compose 3.8 app
Creating app in /path/to/your/app
We're about to launch your Docker Compose app on Fly.io. Here's what you're getting:

Organization: Your Organization
Name:         your-app-name
Region:       Seattle, Washington (this is the fastest region for you)
App Machines: shared-cpu-1x, 1GB RAM
Postgres:     shared-cpu-1x, 1GB RAM, 10GB disk, $19/mo (recommended over containerized database)

✓ Detected multi-container application with 2 services
  Created entrypoint script: fly-entrypoint.sh
  Created machine configuration file: fly.machine.json
  Note: All containers will run in the same VM with shared networking

Your app is ready! Deploy with `flyctl deploy`
```

The scanner automatically:
- Detects that `db` should be replaced with managed Postgres
- Extracts database credentials as encrypted secrets
- Sets up service discovery between `web` and `worker`
- Handles the bind-mounted nginx configuration file
- Creates a single machine deployment (no unnecessary duplication)

## Why This Accelerates Fly.io Adoption

### 1. **Zero Migration Friction**
Developers can deploy existing Docker Compose applications immediately without learning new configuration formats or rewriting service definitions. This removes the biggest barrier to trying Fly.io.

### 2. **Preserves Development Workflows**
Teams can continue using Docker Compose for local development while deploying the same configuration to production on Fly.io. You can test your entire application stack locally with `docker-compose up`, ensure everything works correctly, then deploy the exact same configuration to Fly.io with `flyctl launch`. No context switching between development and deployment tools.

### 3. **Intelligent Infrastructure Recommendations**
Rather than blindly copying Docker Compose services, the scanner recommends Fly.io managed services where appropriate, guiding users toward production-ready architecture patterns.

### 4. **Immediate Edge Benefits**
Applications deployed through Docker Compose support get all of Fly.io's edge benefits—global deployment, automatic HTTPS, and proximity to users—without additional configuration.

### 5. **Gradual Migration Path**
Teams can start with their existing Docker Compose setup and gradually adopt Fly.io-specific features like Machines API, volumes, and advanced networking as they become more comfortable with the platform.

## Advanced Features for Power Users

### Multi-Container Efficiency
The deployment system is smart about multi-container applications, creating only one machine per process group (instead of duplicating containers across multiple machines) for optimal resource utilization.

### Volume and Secret Support
- **Bind mounts** are converted to Fly.io file mounts with proper permissions
- **Named volumes** become Fly.io persistent volumes
- **Docker Compose secrets** are read from files and converted to Fly.io secrets
- **Database credentials** are automatically extracted and encrypted

### Build and Image Flexibility
- Supports both **external images** and **local builds**
- **Single build service** limitation prevents ambiguous build contexts
- **Clean configuration** with no unnecessary build sections for image-only deployments

## Getting Started Today

Docker Compose support is available now in the latest version of `flyctl`. To try it:

1. **Install or update flyctl**:
   ```bash
   curl -L https://fly.io/install.sh | sh
   ```

2. **Navigate to your Docker Compose project**:
   ```bash
   cd your-docker-compose-app/
   ```

3. **Test your app locally first** (optional but recommended):
   ```bash
   docker-compose up
   # Verify your application works correctly
   docker-compose down
   ```

4. **Launch your app**:
   ```bash
   flyctl launch
   ```

5. **Deploy to the edge**:
   ```bash
   flyctl deploy
   ```

That's it! Your multi-container application is now running on Fly.io's global infrastructure with the confidence that it works exactly the same as it did locally.

## Current Limitations and Considerations

While Docker Compose support dramatically simplifies multi-container deployment, there are some important limitations to be aware of:

### Known Constraints
- **Single VM deployment**: All containers run on the same machine with shared resources
- **No horizontal scaling**: Cannot scale individual services independently across multiple machines
- **Single build service**: Only one service can have a build section to prevent ambiguous build contexts
- **Networking simplification**: Docker Compose networks are simplified to localhost communication
- **No privileged containers**: Security limitations prevent privileged mode and host networking

### Areas to Watch
As teams adopt Docker Compose deployments, we're monitoring for potential challenges including:
- **Resource contention**: Multiple containers competing for VM resources
- **Startup dependencies**: Complex dependency chains in large multi-container applications
- **Database coordination**: Migration conflicts and connection pooling in shared database scenarios
- **Performance considerations**: I/O bottlenecks and log volume management
- **Compatibility edge cases**: Variations across Docker Compose versions and image architectures

For most applications, these limitations won't be blockers, but understanding them helps set proper expectations and guides architectural decisions.

## Join the Community

We'd love to hear about your experience deploying Docker Compose applications to Fly.io. Share your success stories, ask questions, or contribute improvements:

- **Community Forum**: [https://community.fly.io](https://community.fly.io)
- **GitHub**: [https://github.com/superfly/flyctl](https://github.com/superfly/flyctl)
- **Documentation**: [https://fly.io/docs/machines/guides-examples/multi-container-machines/](https://fly.io/docs/machines/guides-examples/multi-container-machines/)

The future of application deployment is frictionless migration from development to global edge infrastructure. With Docker Compose support, that future is here today.

---

*Try Docker Compose on Fly.io today and experience the easiest path from local development to global deployment.*