# Chunked Streaming Implementation for Stack Operations

## Overview

This document describes the implementation of chunked transfer encoding for streaming docker compose command output in real-time for the `/api/stacks/{{stackname}}` endpoints.

## Affected Endpoints

The following endpoints now stream docker command output using HTTP chunked transfer encoding:

1. **PUT /api/stacks/{stackname}/start** - Start a stack
2. **PUT /api/stacks/{stackname}/stop** - Stop a stack  
3. **DELETE /api/stacks/{stackname}** - Delete a stack

## Implementation Details

### Core Streaming Function

A new helper function `streamCommandOutput()` was added to handle streaming of docker command output:

```go
func streamCommandOutput(w http.ResponseWriter, cmd *exec.Cmd) error
```

**Features:**
- Streams both stdout and stderr in real-time
- Uses goroutines to handle both streams concurrently
- Flushes output immediately for real-time updates
- Prefixes output with `[STDOUT]` or `[STDERR]` for clarity
- Sends `[ERROR]` prefix for command failures
- Sends `[DONE]` when command completes successfully

### Streamed Commands

The following docker commands are streamed to the HTTP response:

1. **`docker network create`** - Network creation (before compose up/down)
2. **`docker volume create`** - Volume creation (before compose up/down)
3. **`docker compose up -d --wait`** - Stack startup
4. **`docker compose down`** - Stack shutdown/deletion

All commands stream to the same HTTP response in sequence, providing complete visibility of the entire operation.

### Modified Functions

#### 1. HandleDockerComposeFile()

Modified to use `streamCommandOutput()` instead of `cmd.CombinedOutput()` when executing docker compose up/down commands. Also sets streaming headers early and passes ResponseWriter to network/volume creation functions.

**Before:**
```go
output, err := cmd.CombinedOutput()
if err != nil {
    // Error handling with complete output after command finishes
}
```

**After:**
```go
// Set streaming headers early
w.Header().Set("Content-Type", "text/plain; charset=utf-8")
w.Header().Set("Transfer-Encoding", "chunked")
w.WriteHeader(http.StatusOK)

// Stream network/volume creation
ensureNetworksExist(&effectiveCompose, w)
ensureVolumesExist(&effectiveCompose, w)

// Stream docker compose output
if err := streamCommandOutput(w, cmd); err != nil {
    // Error already written to response stream
    return
}
// Response already sent via streaming, so return early
return
```

#### 2. HandleDeleteStack()

Modified to stream docker compose down output and file deletion messages.

**Changes:**
- Sets streaming headers before executing commands
- Streams docker compose down output in real-time
- Sends informational messages about file deletions with `[INFO]` and `[WARN]` prefixes
- Sends final completion message with `[DONE]` prefix

#### 3. ensureNetworksExist()

Modified to accept an optional `http.ResponseWriter` parameter and stream docker network create output.

**Changes:**
- Added `w http.ResponseWriter` parameter
- Streams network creation status messages with `[INFO]` prefix
- Calls `streamCommandOutput()` for `docker network create` commands
- Falls back to non-streaming if ResponseWriter is nil

#### 4. ensureVolumesExist()

Modified to accept an optional `http.ResponseWriter` parameter and stream docker volume create output.

**Changes:**
- Added `w http.ResponseWriter` parameter
- Streams volume creation status messages with `[INFO]` prefix
- Calls `streamCommandOutput()` for `docker volume create` commands
- Falls back to non-streaming if ResponseWriter is nil

## Response Format

### Content Type
```
Content-Type: text/plain; charset=utf-8
Transfer-Encoding: chunked
X-Content-Type-Options: nosniff
```

### Output Format

Each line is prefixed with a tag indicating the source:

- `[STDOUT]` - Standard output from docker command
- `[STDERR]` - Standard error from docker command
- `[ERROR]` - Error message if command fails
- `[DONE]` - Completion message
- `[INFO]` - Informational messages (file operations)
- `[WARN]` - Warning messages (non-critical failures)

### Example Output

```
[INFO] Creating network: dcapi_traefik with driver: bridge
[STDOUT] a1b2c3d4e5f6
[DONE] Command completed successfully
[INFO] Volume already exists: postgres_data
[INFO] Creating network: dcapi_traefik with driver: bridge
[STDOUT] Network dcapi_traefik  Creating
[STDOUT] Network dcapi_traefik  Created
[STDOUT] Container postgres-postgres-1  Creating
[STDOUT] Container postgres-postgres-1  Created
[STDOUT] Container postgres-pgweb-1  Creating
[STDOUT] Container postgres-pgweb-1  Created
[STDOUT] Container postgres-postgres-1  Starting
[STDOUT] Container postgres-postgres-1  Started
[STDOUT] Container postgres-pgweb-1  Starting
[STDOUT] Container postgres-pgweb-1  Started
[DONE] Command completed successfully
```
[STDOUT] Container postgres-pgweb-1  Starting
[STDOUT] Container postgres-pgweb-1  Started
[DONE] Command completed successfully
```

## Client Usage

### Using curl

The `-N` (or `--no-buffer`) flag is required to disable buffering and see streaming output in real-time:

```bash
# Start a stack with streaming output
curl -N -X PUT http://localhost:8080/api/stacks/postgres/start \
  -H "Content-Type: application/yaml" \
  --data-binary @stacks/postgres.yml

# Stop a stack with streaming output
curl -N -X PUT http://localhost:8080/api/stacks/postgres/stop \
  -H "Content-Type: application/yaml" \
  --data-binary @stacks/postgres.yml

# Delete a stack with streaming output
curl -N -X DELETE http://localhost:8080/api/stacks/postgres
```

### Using JavaScript/Fetch API

```javascript
async function startStackWithStreaming(stackName, yamlContent) {
  const response = await fetch(`/api/stacks/${stackName}/start`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/yaml'
    },
    body: yamlContent
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    
    const chunk = decoder.decode(value);
    console.log(chunk); // Process each chunk as it arrives
  }
}
```

### Using Go Client

```go
resp, err := http.Post(url, "application/yaml", body)
if err != nil {
    return err
}
defer resp.Body.Close()

scanner := bufio.NewScanner(resp.Body)
for scanner.Scan() {
    line := scanner.Text()
    fmt.Println(line) // Process each line as it arrives
}
```

## Benefits

1. **Real-time Feedback**: Users see command output as it happens, not after completion
2. **Better UX**: Long-running operations provide immediate feedback
3. **Debugging**: Both stdout and stderr are visible during execution
4. **Progress Tracking**: Users can monitor container creation/startup progress
5. **Timeout Prevention**: Prevents HTTP timeouts on long-running operations

## Backward Compatibility

⚠️ **Breaking Change**: These endpoints now return `text/plain` instead of `application/json` (for delete) or `application/yaml` (for start/stop with actions).

**Note**: The GET and PUT endpoints without actions (ComposeActionNone) still return the YAML file as before.

### Migration Guide

If you have existing clients consuming these endpoints:

1. **Update Content-Type handling**: Expect `text/plain; charset=utf-8` instead of JSON/YAML
2. **Update response parsing**: Parse line-by-line text instead of JSON objects
3. **Enable streaming**: Use appropriate client libraries/flags to handle chunked responses
4. **Update error handling**: Look for `[ERROR]` prefix in stream instead of HTTP error codes

## Testing

A test script is provided at `test-streaming.sh` to demonstrate the streaming functionality:

```bash
./test-streaming.sh
```

This script tests all three streaming endpoints with a sample stack.

## Technical Notes

### Concurrency

The implementation uses goroutines and `sync.WaitGroup` to handle stdout and stderr streams concurrently, ensuring no output is lost and both streams are processed in parallel.

### Flushing

Each line is immediately flushed to the client using `http.Flusher` interface, ensuring real-time delivery without buffering.

### Error Handling

- Command startup errors are returned immediately
- Command execution errors are streamed to the client with `[ERROR]` prefix
- The underlying error is also logged server-side for debugging

### Performance

Streaming has minimal overhead compared to buffering the entire output, and actually improves perceived performance by providing immediate feedback.
