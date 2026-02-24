# Environment Variables and Secrets Loading - Quick Reference

## Overview

dc loads environment variables from **two sources simultaneously**:
- **prod.env** file
- **/run/secrets** directory

Both are merged with **case-insensitive** key matching.

## Key Behaviors

### ✅ Same Key, Same Value
```bash
# prod.env
ADMIN_USERNAME=admin

# /run/secrets/admin_username
admin
```
**Result**: ⚠️ Warning logged, continues normally

### ❌ Same Key, Different Values
```bash
# prod.env
ADMIN_PASSWORD=oldpass

# /run/secrets/admin_password
newpass
```
**Result**: 🔥 **FATAL ERROR** - Application panics

### ✅ Different Keys
```bash
# prod.env
DB_HOST=postgres

# /run/secrets/db_password
secret123
```
**Result**: ✅ Both loaded successfully

## Case Insensitivity

Keys are matched **case-insensitively**:
- `ADMIN_USERNAME` = `admin_username` = `Admin_UserName`
- `DB_PASSWORD` = `db_password` = `DB_Password`

## File Formats

### prod.env Format
```env
# Comments allowed
KEY1=value1
KEY2=value2
```

### /run/secrets Format
- Each filename = variable name
- File content = variable value
- Whitespace trimmed automatically

```bash
/run/secrets/
├── db_password        # Content: "secret123"
├── api_key           # Content: "key456"
└── admin_username    # Content: "admin"
```

## Usage Examples

### Docker Compose with Secrets
```yaml
version: '3.8'
services:
  dc:
    image: dc:latest
    secrets:
      - db_password
      - api_key

secrets:
  db_password:
    external: true
  api_key:
    file: ./secrets/api_key.txt
```

### Checking for Conflicts

```bash
# List all env vars in prod.env
grep -v '^#' ~/.local/containers/prod.env

# List all secrets
ls -la /run/secrets/

# Compare specific key (case-insensitive)
grep -i "ADMIN_USERNAME" ~/.local/containers/prod.env
cat /run/secrets/admin_username
```

## Troubleshooting

### Application Panics on Start

**Error Message**:
```
FATAL: Key 'admin_password' exists in both prod.env (as 'ADMIN_PASSWORD') 
and /run/secrets with DIFFERENT values. prod.env='old***', secrets='new***'
```

**Solution**:
1. Identify the conflict:
   ```bash
   grep -i "admin_password" ~/.local/containers/prod.env
   cat /run/secrets/admin_password
   ```
2. Decide which is correct
3. Update or remove the incorrect one
4. Restart application

### Warning: Duplicate Key

**Log Message**:
```
Warning: Key 'admin_username' exists in both prod.env (as 'ADMIN_USERNAME') 
and /run/secrets with the same value
```

**Solution** (optional cleanup):
- Remove from `prod.env` if using secrets as primary source
- Or remove secret if using `prod.env` as primary source

## Best Practices

1. **✅ DO**: Use `/run/secrets` for sensitive data (passwords, tokens, keys)
2. **✅ DO**: Use `prod.env` for non-sensitive config (hostnames, ports)
3. **✅ DO**: Keep values in sync if duplicating keys
4. **❌ DON'T**: Store secrets in both locations with different values
5. **❌ DON'T**: Commit `prod.env` with sensitive data to version control

## Security

### Password Sanitization
Logs show only first 3 characters:
```
prod.env='pas***', secrets='new***'
```

### File Permissions
```bash
# Recommended
chmod 600 ~/.local/containers/prod.env
chmod 600 /run/secrets/*
```

## Implementation Functions

| Function | Purpose |
|----------|---------|
| `readProdEnv()` | Entry point, reads both sources |
| `readProdEnvWithSecrets()` | Core merging logic |
| `readEnvFile()` | Parse .env format |
| `readSecretsDir()` | Read /run/secrets files |
| `sanitizeForLog()` | Sanitize sensitive values |

## Related Docs

- [SECRETS_LOADING.md](SECRETS_LOADING.md) - Full documentation
- [DOCKER_SECRETS_QUICK_REF.md](DOCKER_SECRETS_QUICK_REF.md) - Docker secrets reference
- [AUTH_QUICK_REFERENCE.md](AUTH_QUICK_REFERENCE.md) - Authentication reference

