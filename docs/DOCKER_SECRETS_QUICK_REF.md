# Docker Secrets - Quick Reference Card

## Priority Order (Highest to Lowest)

1. `ADMIN_USERNAME_FILE` / `ADMIN_PASSWORD_FILE` env vars → custom paths
2. `/run/secrets/ADMIN_USERNAME` / `/run/secrets/ADMIN_PASSWORD` → default Docker secrets
3. `ADMIN_USERNAME` / `ADMIN_PASSWORD` → direct env vars
4. `prod.env` file → fallback

## Quick Setup (30 seconds)

```bash
# Create secrets
mkdir -p secrets && chmod 700 secrets
echo "admin" > secrets/admin_username.txt
echo "$(openssl rand -base64 24)" > secrets/admin_password.txt
chmod 600 secrets/*.txt
echo "secrets/" >> .gitignore
```

## docker-compose.yml (Minimal)

```yaml
version: '3.8'
services:
  dcapi:
    image: dcapi:latest
    environment:
      - ADMIN_USERNAME_FILE=/run/secrets/admin_username
      - ADMIN_PASSWORD_FILE=/run/secrets/admin_password
    secrets: [admin_username, admin_password]
    volumes:
      - /run/user/1000/docker.sock:/var/run/docker.sock
    ports: ["8080:8080"]

secrets:
  admin_username:
    file: ./secrets/admin_username.txt
  admin_password:
    file: ./secrets/admin_password.txt
```

## Test Authentication

```bash
# Get password from file
PASS=$(cat secrets/admin_password.txt)

# Test API
curl -u admin:$PASS http://localhost:8080/api/stacks
```

## Troubleshooting

| Issue | Check |
|-------|-------|
| 401 Unauthorized | `docker exec dcapi ls -la /run/secrets/` |
| Secrets not loading | `docker-compose logs dcapi \| grep -i admin` |
| File not found | Verify `secrets/*.txt` exist and have correct permissions |

## See Also

- Full guide: `docs/DOCKER_SECRETS_AUTH.md`
- Implementation: `docs/DOCKER_SECRETS_IMPLEMENTATION.md`
- Example: `docker-compose.secrets.example.yml`

