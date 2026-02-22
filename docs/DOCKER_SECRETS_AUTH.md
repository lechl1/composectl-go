# Authentication with Docker Secrets - Quick Start

This guide shows you how to use Docker secrets for ComposeCTL authentication.

## Why Use Docker Secrets?

Docker secrets provide a secure way to manage sensitive data:

- ✅ Secrets are never stored in environment variables (visible via `docker inspect`)
- ✅ Secrets are mounted as read-only files in `/run/secrets/`
- ✅ Better security and follows Docker/Kubernetes best practices
- ✅ Easier secret rotation without changing compose files

## Quick Start

### Step 1: Create Secret Files

```bash
# Create a directory for secrets
mkdir -p secrets

# Create username and password files
echo "admin" > secrets/admin_username.txt
echo "your-secure-password-here" > secrets/admin_password.txt

# Secure the files
chmod 600 secrets/*.txt
```

### Step 2: Create docker-compose.yml

```yaml
version: '3.8'

services:
  dcapi:
    image: dcapi:latest
    environment:
      - ADMIN_USERNAME_FILE=/run/secrets/admin_username
      - ADMIN_PASSWORD_FILE=/run/secrets/admin_password
    secrets:
      - admin_username
      - admin_password
    volumes:
      - /run/user/1000/docker.sock:/var/run/docker.sock
    ports:
      - "8080:8080"

secrets:
  admin_username:
    file: ./secrets/admin_username.txt
  admin_password:
    file: ./secrets/admin_password.txt
```

### Step 3: Start the Service

```bash
docker-compose up -d
```

### Step 4: Test Authentication

```bash
# Test API access
curl -u admin:your-secure-password-here http://localhost:8080/api/stacks

# Or open in browser and enter credentials when prompted
open http://localhost:8080
```

## How It Works

ComposeCTL checks for credentials in this priority order:

1. **`ADMIN_USERNAME_FILE` and `ADMIN_PASSWORD_FILE`** - Environment variables pointing to file paths
2. **Default Docker secrets** - `/run/secrets/ADMIN_USERNAME` and `/run/secrets/ADMIN_PASSWORD`
3. **Direct environment variables** - `ADMIN_USERNAME` and `ADMIN_PASSWORD`
4. **prod.env file** - `$HOME/.local/containers/prod.env`

When you set `ADMIN_USERNAME_FILE=/run/secrets/admin_username`, Docker Compose:
1. Reads `secrets/admin_username.txt` from your host
2. Mounts it at `/run/secrets/admin_username` inside the container
3. ComposeCTL reads this file and trims any whitespace/newlines
4. Uses the value for authentication

## Alternative: Using Default Secrets Names

If you name your secrets `ADMIN_USERNAME` and `ADMIN_PASSWORD`, you don't need the `*_FILE` environment variables:

```yaml
version: '3.8'

services:
  dcapi:
    image: dcapi:latest
    secrets:
      - ADMIN_USERNAME
      - ADMIN_PASSWORD
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    ports:
      - "8080:8080"

secrets:
  ADMIN_USERNAME:
    file: ./secrets/username.txt
  ADMIN_PASSWORD:
    file: ./secrets/password.txt
```

ComposeCTL will automatically check `/run/secrets/ADMIN_USERNAME` and `/run/secrets/ADMIN_PASSWORD`.

## Security Best Practices

1. **Never commit secrets to Git**:
   ```bash
   echo "secrets/" >> .gitignore
   ```

2. **Use strong passwords**:
   ```bash
   # Generate a random password
   openssl rand -base64 32 > secrets/admin_password.txt
   ```

3. **Set proper file permissions**:
   ```bash
   chmod 600 secrets/*.txt
   ```

4. **Rotate secrets regularly**:
   ```bash
   # Update the password file
   echo "new-password" > secrets/admin_password.txt
   
   # Restart the service
   docker-compose restart dcapi
   ```

## Troubleshooting

### Check if secrets are mounted correctly

```bash
# Enter the container
docker exec -it dcapi sh

# List secrets
ls -la /run/secrets/

# Read secret content (be careful in production!)
cat /run/secrets/admin_username
```

### View logs for authentication issues

```bash
# Check ComposeCTL logs
docker-compose logs dcapi

# Look for messages like:
# "Loaded ADMIN_USERNAME from file: /run/secrets/admin_username"
# "Loaded ADMIN_PASSWORD from file: /run/secrets/admin_password"
```

### Common Issues

**Problem**: "Warning: ADMIN_USERNAME or ADMIN_PASSWORD not found in any source"

**Solution**: 
- Verify secret files exist: `ls -la secrets/`
- Check docker-compose.yml has correct secrets configuration
- Verify environment variables are set correctly
- Check file permissions: `ls -la secrets/`

**Problem**: Authentication fails but secrets are loaded

**Solution**:
- Check for extra whitespace in secret files
- Verify the password matches exactly
- Ensure files are UTF-8 encoded without BOM

## Docker Swarm

For Docker Swarm deployments, you can create secrets directly:

```bash
# Create secrets in Swarm
echo "admin" | docker secret create admin_username -
echo "your-password" | docker secret create admin_password -

# Deploy stack
docker stack deploy -c docker-compose.yml dcapi
```

docker-compose.yml for Swarm:

```yaml
version: '3.8'

services:
  dcapi:
    image: dcapi:latest
    environment:
      - ADMIN_USERNAME_FILE=/run/secrets/admin_username
      - ADMIN_PASSWORD_FILE=/run/secrets/admin_password
    secrets:
      - admin_username
      - admin_password
    volumes:
      - /run/user/1000/docker.sock:/var/run/docker.sock
    ports:
      - "8080:8080"
    deploy:
      replicas: 1

secrets:
  admin_username:
    external: true
  admin_password:
    external: true
```

## Summary

✅ **Recommended for Production**: Use Docker secrets with `ADMIN_USERNAME_FILE` and `ADMIN_PASSWORD_FILE`

✅ **Easy Setup**: Just create two text files and reference them in docker-compose.yml

✅ **Secure**: Secrets are mounted as read-only files, not exposed in environment variables

✅ **Flexible**: Falls back to environment variables and prod.env for development

For more details, see [SECRETS_MANAGEMENT.md](./SECRETS_MANAGEMENT.md).

