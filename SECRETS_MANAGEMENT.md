# Automatic Secrets Management

## Overview

The composectl-go application now automatically manages Docker Compose secrets when environment variables reference `/run/secrets/` paths. It also automatically generates and manages secret values in a `prod.env` file.

## How It Works

When you define an environment variable with a value pointing to `/run/secrets/XXXX`, the system automatically:

1. **Adds the secret reference to the service** - The secret name `XXXX` is added to the service's `secrets` list (if not already present)
2. **Declares the secret at the top level** - A top-level secret declaration is added with:
   - `name: XXXX`
   - `environment: XXXX`
3. **Manages the secret value in prod.env** - Checks if `XXXX` exists in the `prod.env` file:
   - If missing: Generates a secure 24-character random password
   - If exists: Preserves the existing value
   - Character set: `A-Z`, `a-z`, `0-9`, `._+-`

## Example

### Input YAML

```yaml
services:
  postgres:
    image: postgres
    environment:
      - POSTGRES_DB=mydb
      - POSTGRES_PASSWORD_FILE=/run/secrets/POSTGRES_PASSWORD
    ports:
      - "5432:5432"
```

### After Processing (Effective YAML)

```yaml
services:
  postgres:
    image: postgres
    environment:
      - POSTGRES_DB=mydb
      - POSTGRES_PASSWORD_FILE=/run/secrets/POSTGRES_PASSWORD
    ports:
      - "5432:5432"
    secrets:
      - POSTGRES_PASSWORD  # Automatically added
    # ... other auto-added fields like networks, container_name, etc.

secrets:
  POSTGRES_PASSWORD:      # Automatically added
    name: POSTGRES_PASSWORD
    environment: POSTGRES_PASSWORD
```

### Generated prod.env File

```bash
# Auto-generated secrets for Docker Compose
# This file is managed automatically by composectl
# Do not edit manually unless you know what you are doing

POSTGRES_PASSWORD=aB3.x_Y9+zM2-nK8.qW5_pR7+tC1-uD4.eF6_hG0+
```

The password is:
- 24 characters long
- Randomly generated using cryptographically secure methods
- Contains only safe characters: A-Z, a-z, 0-9, ._+-
- Preserved across updates (not regenerated if it already exists)

## Multiple Secrets

The system handles multiple secrets across multiple services:

```yaml
services:
  app1:
    image: myapp
    environment:
      - API_KEY_FILE=/run/secrets/API_KEY
      - DB_PASSWORD_FILE=/run/secrets/DB_PASSWORD

  app2:
    image: otherapp
    environment:
      - DB_PASSWORD_FILE=/run/secrets/DB_PASSWORD
```

After processing, both services will have the appropriate secrets in their `secrets` list, and both `API_KEY` and `DB_PASSWORD` will be declared at the top level.

The `prod.env` file will contain both secrets:

```bash
# Auto-generated secrets for Docker Compose
# This file is managed automatically by composectl
# Do not edit manually unless you know what you are doing

API_KEY=X9mK.p3_n2+L8-qW5.vR4_zA7+bT1-gH6.yN0_sM2+jF4_
DB_PASSWORD=jK7.mN2_pQ5+rS8-tU1.vW4_xY6+zA9-bC3.dE0_fG2+
```

## When It Applies

This automatic processing happens in two scenarios:

1. **When updating a stack via PUT /api/stacks/{name}** - The `enrichComposeWithTraefikLabels` function processes secrets before saving
2. **When reconstructing from containers** - If a stack YAML doesn't exist and is reconstructed from running containers, secrets are automatically detected and added

## Notes

- Only environment variables with values matching the exact pattern `/run/secrets/XXXX` are processed
- Existing secret declarations are preserved (no duplicates)
- The feature works alongside other automatic enrichments (Traefik labels, networks, volumes, etc.)
- **prod.env Security**: The `prod.env` file contains sensitive passwords and should be:
  - Added to `.gitignore` to prevent committing to version control
  - Protected with appropriate file permissions (e.g., `chmod 600 prod.env`)
  - Backed up securely
- **Password Generation**: Uses `crypto/rand` for cryptographically secure random number generation
- **Idempotent**: Running the process multiple times will not regenerate existing passwords
