# Environment Variable Replacement Without --env-file

## Overview

The dcapi application has been updated to **replace environment variables directly in the Docker Compose YAML content** and pipe it to `docker compose` via stdin, rather than using the `--env-file` argument or setting environment variables on the command.

## Why This Approach?

Previously, the application used `setEnvFromProdEnv()` to set environment variables on the `exec.Command`, which Docker Compose would then use to resolve `${VAR}` placeholders. 

The new approach:
1. ✅ Reads the `prod.env` file
2. ✅ Reads the effective compose YAML file
3. ✅ Replaces all `${VAR}` and `$VAR` placeholders with actual values from `prod.env`
4. ✅ Pipes the fully-resolved YAML content to `docker compose` via stdin using `-f -`

### Benefits:
- **No dependency on --env-file**: The application doesn't use the `--env-file` argument at all
- **No environment variable pollution**: Variables aren't set in the process environment
- **Clear and explicit**: All values are substituted before Docker Compose sees them
- **Portable**: The same approach can be used in shell scripts

## Implementation Details

### New Function: `replaceEnvVarsInYAML()`

Located in `stack.go`, this function:
```go
func replaceEnvVarsInYAML(yamlFilePath string) (string, error)
```

1. Reads the YAML file from disk
2. Reads environment variables from `prod.env`
3. Uses regex to find and replace all `${VAR}` patterns
4. Returns the processed YAML content as a string

The function handles both formats:
- `${VARIABLE_NAME}` - Preferred format
- `$VARIABLE_NAME` - Also supported

### Updated Functions

The following functions now use the new approach:

#### 1. `HandleStopStack()` - Line ~527
Stops a stack by piping processed YAML:
```go
yamlContent, err := replaceEnvVarsInYAML(composeFile)
cmd := exec.Command("docker", "compose", "-f", "-", "-p", stackName, "stop")
cmd.Stdin = strings.NewReader(yamlContent)
```

#### 2. `HandleStartStack()` - Line ~585
Starts a stack by piping processed YAML:
```go
yamlContent, err := replaceEnvVarsInYAML(composeFile)
cmd := exec.Command("docker", "compose", "-f", "-", "-p", stackName, "start")
cmd.Stdin = strings.NewReader(yamlContent)
```

#### 3. `HandleDockerComposeFile()` - Line ~1440
Handles `up` and `down` actions:
```go
yamlContent, err := replaceEnvVarsInYAML(effectiveFilePath)
if action == ComposeActionUp {
    cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, "up", "-d", "--wait")
} else {
    cmd = exec.Command("docker", "compose", "-f", "-", "-p", stackName, "down")
}
cmd.Stdin = strings.NewReader(yamlContent)
```

#### 4. `HandleDeleteStack()` - Line ~2793
Deletes a stack by piping processed YAML:
```go
yamlContent, err := replaceEnvVarsInYAML(composeFile)
cmd := exec.Command("docker", "compose", "-f", "-", "-p", stackName, "down")
cmd.Stdin = strings.NewReader(yamlContent)
```

### Deprecated Function: `setEnvFromProdEnv()`

This function is now marked as deprecated and is no longer called. It has been kept for backward compatibility but is not used in the new implementation.

## Manual Usage via Shell Script

A new script `manual-compose.sh` has been created to demonstrate the same approach from the command line:

### Usage:
```bash
./manual-compose.sh <compose-file> <project-name> <action> [extra-args...]
```

### Examples:

**Start a stack:**
```bash
./manual-compose.sh stacks/postgres.effective.yml postgres up -d --wait
```

**Stop a stack:**
```bash
./manual-compose.sh stacks/postgres.effective.yml postgres down
```

**View config (dry-run):**
```bash
./manual-compose.sh stacks/postgres.effective.yml postgres config
```

### How the Script Works:

1. Reads `prod.env` line by line
2. Exports each variable to the shell environment
3. Uses `envsubst` (if available) or `sed` to replace variables in the YAML
4. Pipes the result to `docker compose -f - -p <project> <action>`

## Example

Given a `prod.env` file:
```env
POSTGRES_ADMIN_PASSWORD=spsGgn-HULC.2q5e7HyjJPju
PGWEB_AUTH_PASSWORD=mypassword123
TZ=UTC
```

And a compose file with:
```yaml
environment:
  - POSTGRES_PASSWORD=${POSTGRES_ADMIN_PASSWORD}
  - TZ=${TZ}
```

The application will:
1. Read both files
2. Replace variables to produce:
```yaml
environment:
  - POSTGRES_PASSWORD=spsGgn-HULC.2q5e7HyjJPju
  - TZ=UTC
```
3. Pipe this to: `docker compose -f - -p postgres up -d`

## Migration Notes

### No Breaking Changes
- The `prod.env` file format remains unchanged
- The effective compose files remain unchanged
- All existing stacks continue to work

### What Changed
- **Internal only**: The method of passing variables to Docker Compose
- **Before**: Set environment variables on the exec.Command
- **After**: Replace variables in YAML and pipe via stdin

### Testing
Build the application:
```bash
go build -o dcapi
```

Test with the manual script:
```bash
./manual-compose.sh stacks/postgres.effective.yml postgres config
```

This should show the fully-resolved YAML with all variables replaced.

## Security Considerations

⚠️ **Important**: The `prod.env` file contains sensitive credentials and should:
- Be added to `.gitignore`
- Have restrictive file permissions (`chmod 600 prod.env`)
- Be backed up securely
- Never be committed to version control

The new approach does not change these security requirements - the `prod.env` file is still the source of truth for all secrets.

## Troubleshooting

### Variables Not Being Replaced
Check that:
1. The variable is defined in `prod.env`
2. The variable name matches exactly (case-sensitive)
3. The YAML uses `${VARIABLE_NAME}` format

### Compose File Parse Errors
If `docker compose` reports YAML syntax errors:
1. Check that the effective YAML file is valid
2. Ensure no variables were left unreplaced (check logs)
3. Test with `docker compose -f <file> config` to validate

### Debug Mode
To see the replaced YAML content before piping to docker compose, check the application logs. The `replaceEnvVarsInYAML()` function logs warnings for any undefined variables.

## Future Enhancements

Possible improvements:
- Add validation to ensure all variables are defined
- Support for default values: `${VAR:-default}`
- Export replaced YAML for debugging: `dcapi export <stack>`
