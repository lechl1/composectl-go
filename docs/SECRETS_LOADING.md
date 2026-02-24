# Environment Variables and Secrets Loading

## Overview

dc now supports loading environment variables from two sources:
1. **prod.env file** (located at `$HOME/.local/containers/prod.env`)
2. **/run/secrets directory** (Docker Swarm secrets location)

Both sources are merged with case-insensitive key matching to ensure compatibility and prevent configuration conflicts.

## How It Works

### Loading Priority

The system reads environment variables from both sources and merges them:

1. **prod.env file**: Traditional `.env` file format with `KEY=VALUE` pairs
2. **/run/secrets directory**: Each file in this directory becomes an environment variable
   - Filename → Variable name
   - File content → Variable value
   - Leading/trailing whitespace is automatically trimmed

### Case-Insensitive Matching

Keys are matched **case-insensitively** across both sources:
- `ADMIN_USERNAME` in prod.env matches `admin_username` in /run/secrets
- `DB_PASSWORD` in prod.env matches `db_password` in /run/secrets

### Conflict Resolution

When the same key (case-insensitive) exists in both sources:

#### **Same Values** → Warning Logged
```
Warning: Key 'admin_username' exists in both prod.env (as 'ADMIN_USERNAME') and /run/secrets with the same value
```
The application continues normally.

#### **Different Values** → Application Panics
```
FATAL: Key 'admin_password' exists in both prod.env (as 'ADMIN_PASSWORD') and /run/secrets with DIFFERENT values. 
prod.env='pas***', secrets='dif***'
```
The application terminates to prevent using inconsistent configuration.

## Usage Examples

### Example 1: Complementary Sources

**prod.env**:
```env
APP_NAME=myapp
DB_HOST=postgres
```

**/run/secrets/db_password**:
```
supersecret123
```

**Result**: All three variables are loaded successfully.

### Example 2: Same Key, Same Value

**prod.env**:
```env
ADMIN_USERNAME=admin
```

**/run/secrets/admin_username**:
```
admin
```

**Result**: 
- Warning is logged about duplicate key
- Value `admin` is used
- Application continues normally

### Example 3: Same Key, Different Values (ERROR)

**prod.env**:
```env
ADMIN_PASSWORD=oldpassword
```

**/run/secrets/admin_password**:
```
newpassword
```

**Result**: 
- Application logs fatal error
- Application panics and terminates
- Prevents using incorrect credentials

## Docker Swarm Integration

This feature integrates seamlessly with Docker Swarm secrets:

```yaml
version: '3.8'

services:
  dc:
    image: dc:latest
    secrets:
      - db_password
      - api_key
    environment:
      - APP_NAME=myapp

secrets:
  db_password:
    external: true
  api_key:
    external: true
```

Secrets are automatically mounted to `/run/secrets/` and loaded by 

## Security Features

### Password Sanitization in Logs

When logging conflicts, sensitive values are sanitized:
- Only the first 3 characters are shown
- Example: `password123` → `pas***`

### Example Log Output
```
FATAL: Key 'DB_PASSWORD' exists in both prod.env (as 'DB_PASSWORD') and /run/secrets with DIFFERENT values. 
prod.env='old***', secrets='new***'
```

## File Formats

### prod.env Format
```env
# Comments are supported
KEY1=value1
KEY2=value2

# Empty lines are ignored
KEY3=value3
```

### /run/secrets Format
Each file in `/run/secrets/` directory:
- Filename is the variable name
- File content is the variable value
- Subdirectories are ignored
- Hidden files (starting with `.`) are ignored

## Error Handling

| Scenario | Behavior |
|----------|----------|
| prod.env missing | No error, continues with empty env vars from this source |
| /run/secrets missing | Logged as info, continues normally |
| Duplicate key, same value | Warning logged, continues |
| Duplicate key, different value | **Fatal error, application panics** |
| Unreadable secret file | Warning logged, file skipped |

## Implementation Details

### Functions

#### `readProdEnv(filePath string) (map[string]string, error)`
Main entry point that calls `readProdEnvWithSecrets` with default `/run/secrets` path.

#### `readProdEnvWithSecrets(prodEnvPath, secretsDir string) (map[string]string, error)`
Core function that:
1. Reads prod.env file
2. Reads /run/secrets directory
3. Performs case-insensitive merging
4. Validates for conflicts

#### `readEnvFile(filePath string) (map[string]string, error)`
Parses a single .env format file.

#### `readSecretsDir(secretsDir string) (map[string]string, error)`
Reads all files from the secrets directory.

#### `sanitizeForLog(value string) string`
Sanitizes sensitive values for logging (shows first 3 chars only).

## Migration Guide

### From prod.env Only

**Before**:
```env
# prod.env
DB_PASSWORD=secret123
API_KEY=key456
```

**After** (using Docker secrets):
```bash
# Create Docker secrets
echo "secret123" | docker secret create db_password -
echo "key456" | docker secret create api_key -

# Remove from prod.env (optional, but recommended)
# prod.env can now be empty or contain non-sensitive config
```

### Gradual Migration

You can migrate gradually:
1. Keep existing prod.env working
2. Add secrets one by one to /run/secrets
3. Ensure values match (or app will panic and tell you)
4. Remove from prod.env once secret is confirmed working

## Best Practices

1. **Use /run/secrets for sensitive data**: Passwords, API keys, tokens
2. **Use prod.env for non-sensitive config**: App names, hostnames, ports
3. **Maintain consistency**: If same key exists in both, ensure same value
4. **Test in development**: Verify secrets load correctly before production
5. **Monitor logs**: Watch for duplicate key warnings

## Troubleshooting

### "FATAL: Key exists in both sources with DIFFERENT values"

**Cause**: Same environment variable (case-insensitive) has different values in prod.env and /run/secrets.

**Solution**:
1. Check both sources:
   ```bash
   cat ~/.local/containers/prod.env | grep -i KEY_NAME
   cat /run/secrets/key_name
   ```
2. Decide which value is correct
3. Update or remove the incorrect one
4. Ensure values match exactly

### "Warning: Duplicate key"

**Cause**: Same key exists in both sources with same value.

**Solution**: This is just informational. To clean up:
1. Remove from prod.env if secret is primary source
2. Or remove secret if prod.env is primary source

### Secrets Not Loading

**Check**:
1. Directory exists: `ls -la /run/secrets`
2. Files are readable: `ls -la /run/secrets/secret_name`
3. Check application logs for error messages

## Related Documentation

- [Docker Secrets Quick Reference](DOCKER_SECRETS_QUICK_REF.md)
- [Docker Secrets Implementation](DOCKER_SECRETS_IMPLEMENTATION.md)
- [Authentication Quick Reference](AUTH_QUICK_REFERENCE.md)

