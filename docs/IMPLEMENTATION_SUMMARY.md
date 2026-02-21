# Implementation Summary: Automatic Secrets Management

## Changes Made

### 1. Updated Data Structures (stack.go)

#### Added ComposeSecret struct
```go
type ComposeSecret struct {
    Name        string `yaml:"name,omitempty"`
    Environment string `yaml:"environment,omitempty"`
    File        string `yaml:"file,omitempty"`
    External    bool   `yaml:"external,omitempty"`
}
```

#### Updated ComposeFile struct
- Added `Secrets map[string]ComposeSecret` field

#### Updated ComposeService struct
- Added `Secrets []string` field

### 2. New Function: processSecrets()

Location: stack.go (around line 1263)

**Purpose**: Automatically detect environment variables with `/run/secrets/` values and ensure proper secret declarations

**Logic**:
1. Scans all environment variables in all services
2. Detects patterns matching `/run/secrets/XXXX`
3. Extracts secret name (XXXX)
4. Adds secret to service's `secrets` list (if not already present)
5. Adds top-level secret declaration with name and environment fields
6. **NEW**: Ensures all secrets exist in `prod.env` file with auto-generated passwords

### 3. New Functions: prod.env Management

#### generateRandomPassword(length int)
- Generates secure random passwords using crypto/rand
- Character set: A-Z, a-z, 0-9, ._+-
- Default length: 24 characters
- Uses cryptographically secure random number generation

#### readProdEnv(filePath string)
- Reads existing prod.env file
- Parses KEY=VALUE pairs
- Skips comments and empty lines
- Returns map of environment variables

#### writeProdEnv(filePath string, envVars map[string]string)
- Writes environment variables to prod.env
- Adds header comments explaining auto-generation
- Sorts keys alphabetically for consistency
- Creates file if it doesn't exist

#### ensureSecretsInProdEnv(secretNames []string)
- Checks if each secret exists in prod.env
- Generates random password for missing secrets
- Preserves existing secret values
- Logs all operations for auditing

### 4. Integration Points

#### enrichAndSanitizeCompose() function
- Added call to `processSecrets(&compose)` at the beginning
- Ensures secrets are processed when updating stacks via API

#### reconstructComposeFromContainers() function
- Added `Secrets: make(map[string]ComposeSecret)` to ComposeFile initialization
- Added call to `processSecrets(&compose)` before marshaling to YAML
- Ensures secrets are detected when reconstructing from running containers

## Behavior

### Automatic Processing
When an environment variable has a value like `/run/secrets/POSTGRES_PASSWORD`:

1. **Service Level**: Adds `POSTGRES_PASSWORD` to the service's `secrets` array
2. **Top Level**: Creates a secret declaration:
   ```yaml
   secrets:
     POSTGRES_PASSWORD:
       name: POSTGRES_PASSWORD
       environment: POSTGRES_PASSWORD
   ```
3. **prod.env File**: Checks if `POSTGRES_PASSWORD` exists in prod.env
   - If missing: Generates a 24-character random password (A-Z, a-z, 0-9, ._+-)
   - If exists: Preserves the existing value
   - Example entry: `POSTGRES_PASSWORD=aB3.x_Y9+zM2-nK8.qW5_pR7+`

### prod.env File Format
```
# Auto-generated secrets for Docker Compose
# This file is managed automatically by composectl
# Do not edit manually unless you know what you are doing

API_KEY=X9mK.p3_n2+L8-qW5.vR4_zA7+bT1-gH6.yN0_sM2+
DB_PASSWORD=aB3.x_Y9+zM2-nK8.qW5_pR7+tC1-uD4.eF6_hG0+
POSTGRES_PASSWORD=jK7.mN2_pQ5+rS8-tU1.vW4_xY6+zA9-bC3.dE0_
```

### Idempotent
- Only adds secrets that don't already exist
- Preserves existing secret declarations
- No duplicates created

### Logging
- Logs each auto-added secret for debugging
- Example: "Auto-added secret 'DB_PASSWORD' to service 'myapp'"
- Example: "Auto-added top-level secret declaration for 'API_KEY'"

## Testing

### Test Files Created
1. `test_secrets.yml` - Simple test case
2. `stacks/test-auto-secrets.yml` - Multi-service test case
3. `test_secrets_main.go` - Standalone test program
4. `SECRETS_MANAGEMENT.md` - User documentation

### Manual Testing Steps
1. Create a stack YAML with environment variables referencing `/run/secrets/`
2. Save via PUT /api/stacks/{name}
3. Verify the effective YAML includes:
   - Secret in service's `secrets` list
   - Top-level `secrets` declaration

## Compatibility

- Works with existing docker-compose.yml files
- Compatible with Docker Compose secret format
- Follows the same pattern as other auto-enrichment features (Traefik labels, networks, etc.)

## Example Use Case

**Before** (user writes):
```yaml
services:
  db:
    image: postgres
    environment:
      - POSTGRES_PASSWORD_FILE=/run/secrets/DB_PASS
```

**After** (system generates):
```yaml
services:
  db:
    image: postgres
    container_name: db
    environment:
      - POSTGRES_PASSWORD_FILE=/run/secrets/DB_PASS
      - TZ=${TZ}
    secrets:
      - DB_PASS
    networks:
      - homelab
    volumes:
      - /etc/localtime:/etc/localtime:ro
      - /etc/timezone:/etc/timezone:ro

networks:
  homelab:
    external: true

secrets:
  DB_PASS:
    name: DB_PASS
    environment: DB_PASS
```
