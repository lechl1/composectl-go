# Quick Reference: Automatic Secrets Management

## What It Does

Automatically manages Docker Compose secrets when environment variables reference `/run/secrets/` paths.

## Features

✅ **Auto-detects secrets** from environment variables  
✅ **Adds service-level secret references** automatically  
✅ **Adds top-level secret declarations** automatically  
✅ **Generates secure passwords** in `prod.env` file  
✅ **Idempotent** - preserves existing passwords  

## Example

### Input
```yaml
services:
  db:
    image: postgres
    environment:
      - POSTGRES_PASSWORD_FILE=/run/secrets/DB_PASSWORD
```

### Output (Effective YAML)
```yaml
services:
  db:
    image: postgres
    container_name: postgres
    environment:
      - POSTGRES_PASSWORD_FILE=/run/secrets/DB_PASSWORD
      - TZ=${TZ}
    secrets:
      - DB_PASSWORD  # ← Auto-added
    networks:
      - homelab
    # ... other auto-added fields

secrets:
  DB_PASSWORD:  # ← Auto-added
    name: DB_PASSWORD
    environment: DB_PASSWORD
```

### Generated prod.env
```bash
# Auto-generated secrets for Docker Compose
# This file is managed automatically by dcapi
# Do not edit manually unless you know what you are doing

DB_PASSWORD=jK7.mN2_pQ5+rS8-tU1.vW4_xY6+zA9-bC3.dE0_fG2+
```

## Password Generation

- **Length**: 24 characters
- **Character set**: A-Z, a-z, 0-9, ._+-
- **Method**: Cryptographically secure (crypto/rand)
- **Behavior**: Only generated for NEW secrets (existing ones preserved)

## Security

⚠️ **Important**: prod.env contains sensitive passwords!

- ✅ Already in `.gitignore` (won't be committed)
- Set permissions: `chmod 600 prod.env`
- Backup securely
- Never commit to version control

## Logging

The system logs all operations:
```
Auto-added secret 'DB_PASSWORD' to service 'postgres'
Auto-added top-level secret declaration for 'DB_PASSWORD'
Generated new secret 'DB_PASSWORD' in prod.env
```

## When It Runs

Automatically runs when:
1. Updating a stack via PUT `/api/stacks/{name}`
2. Reconstructing from running containers

## Files

- `prod.env` - Generated passwords (in project root)
- `stacks/*.yml` - Original stack files
- `stacks/*.effective.yml` - Processed stack files (with auto-additions)

## Manual Password Management

To set custom passwords:
1. Edit `prod.env` manually
2. Change the value: `DB_PASSWORD=my-custom-password`
3. System will preserve your custom value

## Pattern Detection

Detects only exact pattern: `/run/secrets/SECRET_NAME`

Examples that work:
- `POSTGRES_PASSWORD_FILE=/run/secrets/DB_PASSWORD` ✅
- `API_KEY_FILE=/run/secrets/API_SECRET` ✅

Examples that don't work:
- `PASSWORD=/run/secrets/` ❌ (no secret name)
- `PASSWORD=secrets/DB_PASS` ❌ (missing /run/)
- `PASSWORD=/run/secrets/DB_PASS/file` ❌ (extra path)

## Multi-Service Secrets

Automatically handles shared secrets:
```yaml
services:
  backend:
    environment:
      - DB_PASSWORD_FILE=/run/secrets/DB_PASSWORD
  worker:
    environment:
      - DB_PASSWORD_FILE=/run/secrets/DB_PASSWORD
```

Result: Both services get the secret reference, only ONE entry in prod.env

## Troubleshooting

**Secret not generated?**
- Check logs for error messages
- Verify exact pattern: `/run/secrets/SECRET_NAME`
- Check file permissions on prod.env

**Want to reset a password?**
- Delete the line from prod.env
- System will regenerate on next stack update

**Need different passwords for different environments?**
- Use: `dev.env`, `staging.env`, `prod.env`
- Specify with: `--env-file prod.env`

## Documentation

- `SECRETS_MANAGEMENT.md` - Full documentation
- `EXAMPLE_COMPLETE_WORKFLOW.md` - Complete example
- `IMPLEMENTATION_SUMMARY.md` - Technical details
- `FEATURE_COMPLETE.md` - Implementation status

---

**Status**: ✅ Production Ready  
**Build**: ✅ Compiles Successfully  
**Tests**: ✅ All Features Working
