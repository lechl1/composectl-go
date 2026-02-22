# Chunked Streaming - Quick Start Guide

## What is Chunked Streaming?

The `/api/stacks/{stackname}` endpoints now use **HTTP chunked transfer encoding** to stream docker compose command output in real-time. This means you can see the progress of container operations as they happen, rather than waiting for the entire operation to complete.

## Affected Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/stacks/{name}/start` | PUT | Start a stack - streams `docker compose up` output |
| `/api/stacks/{name}/stop` | PUT | Stop a stack - streams `docker compose down` output |
| `/api/stacks/{name}` | DELETE | Delete a stack - streams `docker compose down` + file cleanup |

## Quick Examples

### Using cURL

```bash
# Start a stack with real-time output
curl -N -X PUT http://localhost:8080/api/stacks/postgres/start \
  -H "Content-Type: application/yaml" \
  --data-binary @stacks/postgres.yml

# Stop a stack
curl -N -X PUT http://localhost:8080/api/stacks/postgres/stop \
  -H "Content-Type: application/yaml" \
  --data-binary @stacks/postgres.yml

# Delete a stack
curl -N -X DELETE http://localhost:8080/api/stacks/postgres
```

**Important**: Use the `-N` flag to disable buffering and see output in real-time!

### Using the Bash Test Script

```bash
# Edit test-streaming.sh to set your stack name
./test-streaming.sh
```

### Using the Python Client

```bash
# Start a stack
./streaming-client.py start

# Stop a stack
./streaming-client.py stop

# Delete a stack (with confirmation)
./streaming-client.py delete
```

### Using the Web Demo

1. Start the server: `./dcapi`
2. Open `streaming-demo.html` in your browser
3. Click the buttons to see real-time streaming in action

## Output Format

All output lines are prefixed with tags:

```
[STDOUT] Container creating...       ← Docker standard output (green)
[STDERR] Warning message...          ← Docker standard error (yellow)
[ERROR] Command failed: ...          ← Error occurred (red)
[INFO] File deleted...               ← Informational message (blue)
[WARN] Could not delete file...      ← Warning message (yellow)
[DONE] Command completed             ← Success message (green)
```

### Example Output

```
[STDOUT] Network dcapi_traefik  Creating
[STDOUT] Network dcapi_traefik  Created
[STDOUT] Container postgres-postgres-1  Creating
[STDOUT] Container postgres-postgres-1  Created
[STDOUT] Container postgres-postgres-1  Starting
[STDOUT] Container postgres-postgres-1  Started
[DONE] Command completed successfully
```

## Response Headers

```
Content-Type: text/plain; charset=utf-8
Transfer-Encoding: chunked
X-Content-Type-Options: nosniff
```

## Common Client Patterns

### JavaScript/Fetch

```javascript
const response = await fetch('/api/stacks/mystack/start', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/yaml' },
    body: yamlContent
});

const reader = response.body.getReader();
const decoder = new TextDecoder();

while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    
    const chunk = decoder.decode(value);
    console.log(chunk); // Process each chunk
}
```

### Python/Requests

```python
import requests

response = requests.put(
    'http://localhost:8080/api/stacks/mystack/start',
    data=yaml_content,
    headers={'Content-Type': 'application/yaml'},
    stream=True  # Enable streaming!
)

for line in response.iter_lines():
    if line:
        print(line.decode('utf-8'))
```

### Go

```go
resp, err := http.Post(url, "application/yaml", body)
if err != nil {
    return err
}
defer resp.Body.Close()

scanner := bufio.NewScanner(resp.Body)
for scanner.Scan() {
    fmt.Println(scanner.Text())
}
```

## Benefits

✅ **Real-time Feedback** - See what's happening as it happens  
✅ **No Timeouts** - Long operations won't timeout  
✅ **Better Debugging** - See both stdout and stderr  
✅ **Progress Tracking** - Monitor container startup progress  
✅ **Error Detection** - Catch errors immediately  

## Breaking Changes ⚠️

The response format has changed for these endpoints:

**Before:**
- Content-Type: `application/json` or `application/yaml`
- Response: Complete JSON object or YAML after operation completes

**After:**
- Content-Type: `text/plain; charset=utf-8`
- Response: Streaming text with chunked encoding

**Migration Required**: Update your clients to handle streaming responses.

## Troubleshooting

### Not seeing real-time output?

Make sure your client:
1. Has streaming/buffering disabled
2. Reads the response incrementally (not all at once)
3. Processes chunks as they arrive

### cURL shows nothing until completion?

Add the `-N` flag: `curl -N ...`

### Python requests doesn't stream?

Add `stream=True` parameter: `requests.get(..., stream=True)`

### JavaScript fetch doesn't stream?

Use `response.body.getReader()` to read chunks, don't use `response.text()` or `response.json()`

## Files Reference

| File | Description |
|------|-------------|
| `docs/CHUNKED_STREAMING.md` | Complete technical documentation |
| `STREAMING_IMPLEMENTATION_SUMMARY.md` | Implementation summary |
| `test-streaming.sh` | Bash testing script |
| `streaming-client.py` | Python example client |
| `streaming-demo.html` | Interactive web demo |

## Testing Your Setup

1. **Build the server:**
   ```bash
   go build -o dcapi
   ```

2. **Start the server:**
   ```bash
   ./dcapi
   ```

3. **Test with cURL:**
   ```bash
   curl -N -X PUT http://localhost:8080/api/stacks/postgres/start \
     -H "Content-Type: application/yaml" \
     --data-binary @stacks/postgres.yml
   ```

4. **Watch for streaming output:**
   - You should see `[STDOUT]` lines appearing in real-time
   - Network creation, container creation, and startup messages
   - Final `[DONE]` message when complete

## Next Steps

- Read `docs/CHUNKED_STREAMING.md` for complete documentation
- Try the `streaming-demo.html` for an interactive experience
- Use `streaming-client.py` as a reference for your own clients
- Update your existing clients to handle streaming responses

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review the complete documentation in `docs/CHUNKED_STREAMING.md`
3. Examine the example clients for reference implementations
