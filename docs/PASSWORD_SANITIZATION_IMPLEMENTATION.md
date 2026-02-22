# Password Sanitization Implementation Summary

## Overview
Implemented plaintext password detection and sanitization for reconstructed YAMLs in the PUT /api/stacks/{{stackname}} endpoint.

## Changes Made

### 1. New Function: `sanitizeComposePasswords`
**Location:** stack.go (after `normalizeEnvKey` function, around line 698)

```go
// sanitizeComposePasswords sanitizes environment variables in a ComposeFile
// by extracting plaintext passwords to prod.env and replacing them with variable references ${ENV_KEY}
func sanitizeComposePasswords(compose *ComposeFile) {
	const prodEnvPath = "prod.env"
	
	// Read existing prod.env
	envVars, err := readProdEnv(prodEnvPath)
	if err != nil {
		log.Printf("Warning: Failed to read prod.env: %v", err)
		envVars = make(map[string]string)
	}
	
	modified := false
	
	// Process each service and extract passwords
	for serviceName, service := range compose.Services {
		envArray := normalizeEnvironment(service.Environment)
		var sanitizedEnv []string
		for _, envVar := range envArray {
			// Extract sensitive values to prod.env
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 && isSensitiveKey(parts[0]) && parts[1] != "" {
				normalizedKey := normalizeEnvKey(parts[0])
				if _, exists := envVars[normalizedKey]; !exists {
					envVars[normalizedKey] = parts[1]
					modified = true
				}
			}
			// Sanitize the environment variable
			sanitizedEnv = append(sanitizedEnv, sanitizeEnvironmentVariable(envVar))
		}
		service.Environment = sanitizedEnv
		compose.Services[serviceName] = service
	}
	
	// Write back to prod.env if modified
	if modified {
		writeProdEnv(prodEnvPath, envVars)
	}
}
```

**Purpose:** This function takes a ComposeFile structure and:
1. Extracts plaintext passwords from environment variables
2. Saves them to `prod.env` with normalized key names
3. Replaces plaintext values with variable references `${NORMALIZED_KEY}`

### 2. Updated Function: `HandlePutStack`
**Location:** stack.go (around line 1060-1160)

**Changes:**
- Added password sanitization step before writing the original `.yml` file
- The flow is now:
  1. Parse and enrich YAML (for `.effective.yml`) - contains plaintext passwords
  2. Parse and sanitize YAML (for `.yml`) - passwords replaced with `${VAR_NAME}`
  3. Write sanitized version to `.yml` file
  4. Write enriched version (with plaintext) to `.effective.yml` file

**Code added:**
```go
// Sanitize passwords in the original YAML for the .yml file
var sanitizedCompose ComposeFile
if err := yaml.Unmarshal(body, &sanitizedCompose); err != nil {
	log.Printf("Error parsing YAML for sanitization: %v", err)
	http.Error(w, fmt.Sprintf("Failed to parse YAML: %v", err), http.StatusBadRequest)
	return
}
sanitizeComposePasswords(&sanitizedCompose)

// Marshal the sanitized version back to YAML
var sanitizedBuf strings.Builder
sanitizedEncoder := yaml.NewEncoder(&sanitizedBuf)
sanitizedEncoder.SetIndent(2)
if err := sanitizedEncoder.Encode(sanitizedCompose); err != nil {
	log.Printf("Error encoding sanitized YAML: %v", err)
	http.Error(w, fmt.Sprintf("Failed to encode YAML: %v", err), http.StatusInternalServerError)
	return
}
if err := sanitizedEncoder.Close(); err != nil {
	log.Printf("Error closing sanitized YAML encoder: %v", err)
	http.Error(w, fmt.Sprintf("Failed to encode YAML: %v", err), http.StatusInternalServerError)
	return
}
sanitizedYAML := []byte(sanitizedBuf.String())
```

## How It Works

### Password Detection
The existing `sanitizeEnvironmentVariable` function detects sensitive environment variables by checking if the key contains any of these keywords:
- PASSWD
- PASSWORD
- SECRET
- KEY
- TOKEN
- API_KEY
- APIKEY
- PRIVATE

### Password Replacement
When a sensitive variable is detected:
1. The plaintext password value is extracted and saved to `prod.env` with the normalized key
2. The key is normalized using `normalizeEnvKey` (converts to uppercase, replaces special chars with underscores)
3. The value in the YAML is replaced with `${NORMALIZED_KEY}`

**Example:**
```yaml
# Before sanitization:
- GITEA__database__PASSWD=awefsdzfasf
- MY_API_KEY=super_secret_123

# After sanitization (in .yml file):
- GITEA__database__PASSWD=${GITEA_DATABASE_PASSWD}
- MY_API_KEY=${MY_API_KEY}
```

**prod.env file:**
```bash
# Auto-generated secrets for Docker Compose
# This file is managed automatically by dcapi
# Do not edit manually unless you know what you are doing

GITEA_DATABASE_PASSWD=awefsdzfasf
MY_API_KEY=super_secret_123
```

## File Structure After PUT Request

### stackname.yml (Original File)
- Contains sanitized environment variables
- Passwords are replaced with variable references like `${PASSWORD}`
- Safe to commit to version control
- User-facing file

### stackname.effective.yml (Effective File)
- Contains plaintext passwords
- Includes all enrichments (Traefik labels, networks, volumes, etc.)
- Used by Docker Compose for actual deployment
- Should be in .gitignore

## Reuse of Existing Functions

This implementation reuses the existing password detection logic:
1. **`sanitizeEnvironmentVariable(envStr string)`** - Already used in `reconstructComposeFromContainers`
2. **`normalizeEnvKey(key string)`** - Already used for key normalization
3. **`normalizeEnvironment(env interface{})`** - Already used to convert environment variables to string array

## Benefits

1. **Consistent behavior:** Both reconstructed YAMLs (from containers) and PUT endpoint now use the same sanitization logic
2. **Security:** Plaintext passwords are never stored in the user-facing `.yml` files
3. **Centralized secrets management:** All passwords are stored in `prod.env` for easy management and backup
4. **Functionality:** The `.effective.yml` file retains plaintext passwords for Docker Compose deployment
5. **Minimal code changes:** Reused existing functions rather than duplicating logic
6. **Backward compatible:** Existing stacks continue to work as before
7. **No duplicate passwords:** Existing passwords in `prod.env` are preserved and not overwritten

## Testing

To test the implementation:
1. Start the dcapi server
2. Send a PUT request with a YAML containing plaintext passwords
3. Verify that `.yml` has sanitized passwords
4. Verify that `.effective.yml` has plaintext passwords
5. Verify that `prod.env` contains the extracted passwords

Example:
```bash
curl -X PUT http://localhost:8080/api/stacks/test \
  -H "Content-Type: text/plain" \
  --data-binary @- <<EOF
services:
  db:
    image: postgres
    environment:
      - POSTGRES_PASSWORD=secret123
      - POSTGRES_USER=admin
EOF

# Check the files
cat stacks/test.yml              # Should have ${POSTGRES_PASSWORD}
cat stacks/test.effective.yml    # Should have secret123
cat prod.env                      # Should have POSTGRES_PASSWORD=secret123

# Test enrichment endpoint (without side effects)
curl -X POST http://localhost:8080/api/enrich \
  -H "Content-Type: text/plain" \
  --data-binary @- <<EOF
services:
  db:
    image: postgres
    environment:
      - POSTGRES_PASSWORD=secret456
EOF
```

## Related Files

- `stack.go` - Main implementation file
- `stacks/*.yml` - Original stack files (sanitized)
- `stacks/*.effective.yml` - Effective stack files (with plaintext)
- `prod.env` - Contains actual password values for Docker Compose secrets

## Notes

- The `.effective.yml` file is what Docker Compose actually uses
- The `.yml` file is the user-editable version with sanitized secrets
- The `prod.env` file stores all extracted passwords with normalized keys
- When a PUT request is made, plaintext passwords are automatically extracted to `prod.env` before sanitization
- Existing passwords in `prod.env` are never overwritten - they are preserved across updates
- This follows the same pattern already established for secrets management and file reconstruction
- The `prod.env` file should be added to `.gitignore` and backed up securely
