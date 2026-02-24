# Runtime Environment Variable Implementation

## Overview

This document describes the implementation of runtime environment variable support in  The system now prioritizes environment variables available from the runtime environment over those stored in `prod.env`.

## Changes Made

### 1. Modified `ensureSecretsInProdEnv()` Function

**Location**: `stack.go` (lines ~2098-2144)

**Changes**:
- Added runtime environment check using `os.Getenv()` before generating secrets
- If a secret exists in the runtime environment, it is NOT stored in `prod.env`
- Logs when secrets are sourced from runtime vs. generated

**Behavior**:
```go
// Check if variable is available in runtime environment
if runtimeValue := os.Getenv(secretName); runtimeValue != "" {
    log.Printf("Secret '%s' is available from runtime environment, skipping prod.env", secretName)
    continue
}
```

### 2. Modified `sanitizeComposePasswords()` Function

**Location**: `stack.go` (lines ~803-1070)

**Changes Applied to Multiple Sections**:

#### A. Service Environment Variables
- Checks `os.Getenv()` for sensitive passwords before storing to `prod.env`
- Checks `os.Getenv()` for variable references before storing to `prod.env`
- Logs when variables are sourced from runtime

#### B. Service Labels
- Checks `os.Getenv()` before extracting variables from labels
- Applies to both array and map label formats

#### C. Configs
- Checks `os.Getenv()` before extracting variables from config content
- Checks `os.Getenv()` before extracting variables from config file paths

#### D. Volumes
- Checks `os.Getenv()` before extracting variables from volume names
- Checks `os.Getenv()` before extracting variables from volume driver options

#### E. Service-Level Fields
- Volume mounts: Checks runtime environment first
- Command field: Checks runtime environment first
- Image field: Checks runtime environment first

**Pattern Used Throughout**:
```go
// Check if variable is available in runtime environment
if runtimeValue := os.Getenv(normalizedVarName); runtimeValue != "" {
    log.Printf("Environment variable '%s' is available from runtime environment, skipping prod.env", normalizedVarName)
} else if _, exists := envVars[normalizedVarName]; !exists {
    // Only add if not already in prod.env and not in runtime
    envVars[normalizedVarName] = value
    modified = true
    log.Printf("Added environment variable '%s' to prod.env...", normalizedVarName)
}
```

### 3. Modified `replaceEnvVarsInYAML()` Function

**Location**: `stack.go` (lines ~2176-2234)

**Changes**:
- Prioritizes runtime environment over `prod.env` when replacing variables
- Checks `os.Getenv()` first, then falls back to `prod.env`
- Updated warning messages to indicate both runtime and prod.env were checked

**Behavior for `${VAR}` format**:
```go
// Check runtime environment first, then prod.env
if runtimeValue := os.Getenv(varName); runtimeValue != "" {
    return runtimeValue
}
if value, exists := envVars[varName]; exists {
    return value
}
log.Printf("Warning: Environment variable %s not found in runtime or prod.env", varName)
```

**Behavior for `$VAR` format**:
```go
// Check runtime environment first, then prod.env
if runtimeValue := os.Getenv(varName); runtimeValue != "" {
    return runtimeValue + trailingChar
}
if value, exists := envVars[varName]; exists {
    return value + trailingChar
}
log.Printf("Warning: Environment variable %s not found in runtime or prod.env", varName)
```

## Use Cases

### Use Case 1: Runtime Secrets for CI/CD

**Scenario**: Running dcapi in a CI/CD pipeline with secrets injected as environment variables.

**Before**:
- All secrets would be written to `prod.env` file
- Risk of secrets being committed or leaked

**After**:
- Secrets available from runtime are NOT written to `prod.env`
- Secrets are used directly from environment
- Log messages indicate source of secrets

**Example**:
```bash
# In CI/CD pipeline
export POSTGRES_PASSWORD="ci-secret-123"
export API_KEY="github-actions-secret"

# Run dcapi
./dcapi

# Result: POSTGRES_PASSWORD and API_KEY are NOT added to prod.env
# They are used directly from environment variables
```

### Use Case 2: Docker Container Deployment

**Scenario**: Running dcapi as a Docker container with secrets passed via environment variables.

**Before**:
- Container would create `prod.env` with duplicate secrets
- No clear separation between container env and file-based config

**After**:
- Container uses runtime environment variables
- Only missing variables are generated and stored in `prod.env`
- Clear logging shows which variables come from where

**Example**:
```bash
docker run -e DATABASE_PASSWORD=secret123 \
           -e PUBLIC_DOMAIN=example.com \
           dcapi:latest

# Result: DATABASE_PASSWORD and PUBLIC_DOMAIN from container environment
# Other missing variables generated in prod.env
```

### Use Case 3: Kubernetes Deployments

**Scenario**: Running in Kubernetes with ConfigMaps and Secrets.

**Before**:
- K8s secrets would be duplicated in `prod.env`
- Potential security issues with file persistence

**After**:
- K8s-provided environment variables are used directly
- No duplication in `prod.env`
- Follows Kubernetes best practices

**Example**:
```yaml
# Kubernetes Deployment
env:
  - name: DB_PASSWORD
    valueFrom:
      secretKeyRef:
        name: db-secret
        key: password
  - name: DOMAIN_NAME
    valueFrom:
      configMapKeyRef:
        name: app-config
        key: domain

# Result: DB_PASSWORD and DOMAIN_NAME from K8s, not prod.env
```

### Use Case 4: Development vs Production

**Scenario**: Different configurations for dev and prod environments.

**Before**:
- Had to manage separate `prod.env` files for each environment
- Risk of using wrong file

**After**:
- Set environment-specific variables in runtime
- Single `prod.env` for common/generated values
- Clear separation of concerns

**Example**:
```bash
# Development
export PUBLIC_DOMAIN="localhost"
export DEBUG_MODE="true"
./dcapi

# Production
export PUBLIC_DOMAIN="example.com"
export DEBUG_MODE="false"
./dcapi

# Result: Environment-specific values from runtime
# Common generated secrets in prod.env
```

## Priority Order

When resolving environment variables, dcapi now follows this priority:

1. **Runtime Environment** (`os.Getenv()`) - HIGHEST PRIORITY
2. **prod.env file** - Fallback
3. **Empty string** - If not found in either location

This applies to:
- Variable replacement in YAML files
- Secret generation
- Password sanitization
- Environment variable extraction

## Logging

New log messages help track the source of environment variables:

```
Secret 'DATABASE_PASSWORD' is available from runtime environment, skipping prod.env
Environment variable 'PUBLIC_DOMAIN' is available from runtime environment, skipping prod.env
Warning: Environment variable API_KEY not found in runtime or prod.env
Generated new secret 'RANDOM_SECRET' in prod.env
```

## Backward Compatibility

✅ **Fully backward compatible**:
- If no runtime environment variables are set, behavior is identical to before
- Existing `prod.env` files continue to work
- No breaking changes to API or file formats

## Security Benefits

1. **Reduced Secret Storage**: Secrets don't need to be written to disk if available from runtime
2. **CI/CD Friendly**: Works seamlessly with secret injection systems
3. **Container Native**: Follows container best practices for secret management
4. **Kubernetes Compatible**: Integrates well with K8s ConfigMaps and Secrets
5. **Audit Trail**: Clear logging shows where each variable originates

## Testing

To test the new functionality:

1. Set an environment variable before running dcapi:
   ```bash
   export TEST_VAR="from_runtime"
   ```

2. Create a compose file that references `${TEST_VAR}`

3. Run dcapi and observe logs:
   - Should see: "Environment variable 'TEST_VAR' is available from runtime environment, skipping prod.env"
   - `TEST_VAR` should NOT appear in `prod.env`
   - YAML should be processed with the runtime value

4. Check the effective YAML - it should contain the runtime value

## Migration Guide

No migration needed! The changes are transparent to existing deployments.

**Optional optimization**: If you have environment variables in both runtime and `prod.env`, you can:
1. Remove them from `prod.env` (they'll be sourced from runtime)
2. Keep them in `prod.env` as fallback (runtime takes precedence)

## Summary

This implementation makes dcapi more flexible and secure by:
- ✅ Supporting runtime environment variables
- ✅ Avoiding duplicate secret storage
- ✅ Following container and cloud-native best practices
- ✅ Maintaining full backward compatibility
- ✅ Providing clear logging for debugging

The system is now production-ready for CI/CD, Docker, Kubernetes, and other cloud-native environments.
