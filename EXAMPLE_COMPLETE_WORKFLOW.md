# Complete Example: Automatic Secrets Management

This document demonstrates the complete workflow of automatic secrets management in composectl-go.

## Step 1: Create a Stack YAML

Create a file `stacks/myapp.yml`:

```yaml
services:
  database:
    image: postgres:15
    environment:
      - POSTGRES_DB=myapp
      - POSTGRES_USER=myappuser
      - POSTGRES_PASSWORD_FILE=/run/secrets/DB_PASSWORD
    ports:
      - "5432:5432"

  redis:
    image: redis:7
    environment:
      - REDIS_PASSWORD_FILE=/run/secrets/REDIS_PASSWORD
    ports:
      - "6379:6379"

  backend:
    image: myapp/backend:latest
    environment:
      - DB_PASSWORD_FILE=/run/secrets/DB_PASSWORD
      - REDIS_PASSWORD_FILE=/run/secrets/REDIS_PASSWORD
      - API_SECRET_FILE=/run/secrets/API_SECRET
    ports:
      - "8080:8080"
```

## Step 2: System Processes the Stack

When you save this via PUT `/api/stacks/myapp`, the system automatically:

### 2a. Adds Service-Level Secrets

```yaml
services:
  database:
    # ... existing fields ...
    secrets:
      - DB_PASSWORD  # ← Automatically added

  redis:
    # ... existing fields ...
    secrets:
      - REDIS_PASSWORD  # ← Automatically added

  backend:
    # ... existing fields ...
    secrets:
      - DB_PASSWORD      # ← Automatically added
      - REDIS_PASSWORD   # ← Automatically added
      - API_SECRET       # ← Automatically added
```

### 2b. Adds Top-Level Secret Declarations

```yaml
secrets:
  API_SECRET:
    name: API_SECRET
    environment: API_SECRET
  DB_PASSWORD:
    name: DB_PASSWORD
    environment: DB_PASSWORD
  REDIS_PASSWORD:
    name: REDIS_PASSWORD
    environment: REDIS_PASSWORD
```

### 2c. Creates/Updates prod.env File

The system checks `prod.env` and creates entries for missing secrets:

```bash
# Auto-generated secrets for Docker Compose
# This file is managed automatically by composectl
# Do not edit manually unless you know what you are doing

API_SECRET=K8pL.mN3_qR7+sT2-uV9.wX1_yZ5+aB6-cD4.eF0_gH2+
DB_PASSWORD=jK7.mN2_pQ5+rS8-tU1.vW4_xY6+zA9-bC3.dE0_fG2+
REDIS_PASSWORD=pQ3.rS9_tU6+vW2-xY8.zA1_bC7+dE4-fG0.hI5_jK3+
```

## Step 3: Effective YAML

The final effective YAML (`myapp.effective.yml`) includes all auto-added features:

```yaml
services:
  database:
    image: postgres:15
    container_name: postgres
    environment:
      - POSTGRES_DB=myapp
      - POSTGRES_USER=myappuser
      - POSTGRES_PASSWORD_FILE=/run/secrets/DB_PASSWORD
      - TZ=${TZ}
    ports:
      - "5432:5432"
    secrets:
      - DB_PASSWORD
    networks:
      - homelab
    volumes:
      - /etc/localtime:/etc/localtime:ro
      - /etc/timezone:/etc/timezone:ro

  redis:
    image: redis:7
    container_name: redis
    environment:
      - REDIS_PASSWORD_FILE=/run/secrets/REDIS_PASSWORD
      - TZ=${TZ}
    ports:
      - "6379:6379"
    secrets:
      - REDIS_PASSWORD
    networks:
      - homelab
    volumes:
      - /etc/localtime:/etc/localtime:ro
      - /etc/timezone:/etc/timezone:ro

  backend:
    image: myapp/backend:latest
    container_name: backend
    environment:
      - DB_PASSWORD_FILE=/run/secrets/DB_PASSWORD
      - REDIS_PASSWORD_FILE=/run/secrets/REDIS_PASSWORD
      - API_SECRET_FILE=/run/secrets/API_SECRET
      - TZ=${TZ}
    ports:
      - "8080:8080"
    secrets:
      - DB_PASSWORD
      - REDIS_PASSWORD
      - API_SECRET
    networks:
      - homelab
    volumes:
      - /etc/localtime:/etc/localtime:ro
      - /etc/timezone:/etc/timezone:ro
    labels:
      - traefik.enable=true
      - traefik.http.routers.backend.entrypoints=https
      - traefik.http.routers.backend.rule=Host(`backend.localhost`) || Host(`backend.${PUBLIC_DOMAIN_NAME}`) || Host(`backend.leochl.ddns.net`) || Host(`backend`)
      - traefik.http.routers.backend.service=backend
      - traefik.http.routers.backend.tls=true
      - traefik.http.services.backend.loadbalancer.server.port=8080
      - traefik.http.services.backend.loadbalancer.server.scheme=http

networks:
  homelab:
    external: true

secrets:
  API_SECRET:
    name: API_SECRET
    environment: API_SECRET
  DB_PASSWORD:
    name: DB_PASSWORD
    environment: DB_PASSWORD
  REDIS_PASSWORD:
    name: REDIS_PASSWORD
    environment: REDIS_PASSWORD
```

## Step 4: Deployment

When you deploy with Docker Compose, the secrets are automatically loaded from the environment:

```bash
# Docker Compose reads from prod.env
docker compose --env-file prod.env -f myapp.effective.yml -p myapp up -d
```

The containers will have access to the secrets via `/run/secrets/` paths.

## Key Features Demonstrated

1. ✅ **Automatic secret detection** from environment variables
2. ✅ **Service-level secret references** added automatically
3. ✅ **Top-level secret declarations** added automatically
4. ✅ **prod.env file management** with secure random passwords
5. ✅ **Idempotent operations** - existing secrets are preserved
6. ✅ **Integration with other features** - Traefik labels, networks, volumes, etc.

## Security Best Practices

1. **Never commit prod.env to version control**
   - Already added to `.gitignore`
   
2. **Set proper file permissions**
   ```bash
   chmod 600 prod.env
   ```

3. **Backup prod.env securely**
   - Store in a password manager or encrypted backup

4. **Rotate passwords periodically**
   - Manually edit prod.env to update passwords
   - Existing values are never overwritten automatically

5. **Use different secrets for different environments**
   - dev.env, staging.env, prod.env
