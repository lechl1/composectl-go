# Migration Guide: Upgrading to Chunked Streaming

## Overview

The `/api/stacks/{stackname}` endpoints now use chunked transfer encoding to stream docker compose output in real-time. This guide will help you update your existing clients.

## What Changed?

### Before (Buffered Response)
```javascript
// Old approach - waits for complete response
const response = await fetch('/api/stacks/mystack/start', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/yaml' },
    body: yamlContent
});

const yaml = await response.text(); // Complete YAML file
console.log(yaml);
```

### After (Streaming Response)
```javascript
// New approach - streams output in real-time
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
    console.log(chunk); // Real-time output
}
```

## Response Format Changes

### Affected Endpoints

| Endpoint | Old Content-Type | New Content-Type | Old Format | New Format |
|----------|------------------|------------------|------------|------------|
| `PUT /api/stacks/{name}/start` | application/yaml | text/plain | YAML file | Streaming text |
| `PUT /api/stacks/{name}/stop` | application/yaml | text/plain | YAML file | Streaming text |
| `DELETE /api/stacks/{name}` | application/json | text/plain | JSON object | Streaming text |

### Output Format

Old responses returned complete data after operation:
```yaml
# YAML response (start/stop)
services:
  myservice:
    image: nginx
```

```json
// JSON response (delete)
{
  "success": true,
  "message": "Stack deleted"
}
```

New responses stream tagged output lines:
```
[STDOUT] Network creating...
[STDOUT] Container starting...
[DONE] Command completed successfully
```

## Migration by Language/Framework

### JavaScript/TypeScript (Browser)

#### Old Code
```javascript
async function startStack(stackName, yaml) {
    const response = await fetch(`/api/stacks/${stackName}/start`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/yaml' },
        body: yaml
    });
    
    const result = await response.text();
    return result;
}
```

#### New Code
```javascript
async function startStack(stackName, yaml, onProgress) {
    const response = await fetch(`/api/stacks/${stackName}/start`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/yaml' },
        body: yaml
    });
    
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    
    while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        
        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');
        
        for (const line of lines) {
            if (line.trim()) {
                onProgress(line); // Callback for each line
            }
        }
    }
}

// Usage
await startStack('mystack', yamlContent, (line) => {
    console.log(line);
    // Update UI with progress
});
```

### Node.js

#### Old Code
```javascript
const axios = require('axios');

async function startStack(stackName, yaml) {
    const response = await axios.put(
        `/api/stacks/${stackName}/start`,
        yaml,
        { headers: { 'Content-Type': 'application/yaml' } }
    );
    return response.data;
}
```

#### New Code
```javascript
const axios = require('axios');

async function startStack(stackName, yaml, onProgress) {
    const response = await axios.put(
        `/api/stacks/${stackName}/start`,
        yaml,
        {
            headers: { 'Content-Type': 'application/yaml' },
            responseType: 'stream' // Enable streaming
        }
    );
    
    return new Promise((resolve, reject) => {
        response.data.on('data', (chunk) => {
            const lines = chunk.toString().split('\n');
            lines.forEach(line => {
                if (line.trim()) onProgress(line);
            });
        });
        
        response.data.on('end', () => resolve());
        response.data.on('error', reject);
    });
}

// Usage
await startStack('mystack', yamlContent, (line) => {
    console.log(line);
});
```

### Python

#### Old Code
```python
import requests

def start_stack(stack_name, yaml_content):
    response = requests.put(
        f'/api/stacks/{stack_name}/start',
        data=yaml_content,
        headers={'Content-Type': 'application/yaml'}
    )
    return response.text
```

#### New Code
```python
import requests

def start_stack(stack_name, yaml_content, on_progress):
    response = requests.put(
        f'/api/stacks/{stack_name}/start',
        data=yaml_content,
        headers={'Content-Type': 'application/yaml'},
        stream=True  # Enable streaming
    )
    
    for line in response.iter_lines():
        if line:
            decoded_line = line.decode('utf-8')
            on_progress(decoded_line)

# Usage
def print_progress(line):
    print(line)

start_stack('mystack', yaml_content, print_progress)
```

### Go

#### Old Code
```go
func startStack(stackName string, yaml []byte) (string, error) {
    resp, err := http.Post(
        fmt.Sprintf("/api/stacks/%s/start", stackName),
        "application/yaml",
        bytes.NewReader(yaml),
    )
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    return string(body), err
}
```

#### New Code
```go
func startStack(stackName string, yaml []byte, onProgress func(string)) error {
    resp, err := http.Post(
        fmt.Sprintf("/api/stacks/%s/start", stackName),
        "application/yaml",
        bytes.NewReader(yaml),
    )
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Text()
        onProgress(line)
    }
    
    return scanner.Err()
}

// Usage
err := startStack("mystack", yamlContent, func(line string) {
    fmt.Println(line)
})
```

### cURL

#### Old Code
```bash
curl -X PUT http://localhost:8080/api/stacks/postgres/start \
  -H "Content-Type: application/yaml" \
  --data-binary @stacks/postgres.yml
```

#### New Code
```bash
# Add -N flag to disable buffering
curl -N -X PUT http://localhost:8080/api/stacks/postgres/start \
  -H "Content-Type: application/yaml" \
  --data-binary @stacks/postgres.yml
```

## Parsing the Output

### Line Prefixes

Each line has a prefix indicating its type:

```
[STDOUT] - Docker standard output
[STDERR] - Docker standard error
[ERROR]  - Command failure
[DONE]   - Success completion
[INFO]   - Informational message
[WARN]   - Warning message
```

### Example Parser

```javascript
function parseStreamLine(line) {
    const patterns = {
        stdout: /^\[STDOUT\] (.+)$/,
        stderr: /^\[STDERR\] (.+)$/,
        error: /^\[ERROR\] (.+)$/,
        done: /^\[DONE\] (.+)$/,
        info: /^\[INFO\] (.+)$/,
        warn: /^\[WARN\] (.+)$/
    };
    
    for (const [type, pattern] of Object.entries(patterns)) {
        const match = line.match(pattern);
        if (match) {
            return {
                type: type,
                message: match[1],
                raw: line
            };
        }
    }
    
    return { type: 'unknown', message: line, raw: line };
}

// Usage
const parsed = parseStreamLine('[STDOUT] Container starting...');
console.log(parsed.type);    // 'stdout'
console.log(parsed.message); // 'Container starting...'
```

## Handling Errors

### Old Approach
```javascript
try {
    const response = await fetch(url);
    if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
    }
    const data = await response.text();
} catch (error) {
    console.error('Request failed:', error);
}
```

### New Approach
```javascript
try {
    const response = await fetch(url);
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    
    let hasError = false;
    
    while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        
        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');
        
        for (const line of lines) {
            if (line.startsWith('[ERROR]')) {
                hasError = true;
                console.error(line);
            } else if (line.startsWith('[DONE]')) {
                console.log('Operation completed successfully');
            }
        }
    }
    
    if (hasError) {
        throw new Error('Operation failed - check logs');
    }
} catch (error) {
    console.error('Request failed:', error);
}
```

## Testing Your Migration

### 1. Update Your Client Code
Follow the examples above for your language/framework.

### 2. Test with a Simple Endpoint
Start with the start endpoint to verify streaming works:

```bash
curl -N -X PUT http://localhost:8080/api/stacks/postgres/start \
  -H "Content-Type: application/yaml" \
  --data-binary @stacks/postgres.yml
```

### 3. Verify Real-time Output
You should see lines appearing progressively, not all at once.

### 4. Check Error Handling
Test with an invalid stack to ensure errors are handled:

```bash
curl -N -X DELETE http://localhost:8080/api/stacks/nonexistent
```

## Common Issues

### Issue: Not seeing real-time output

**Cause:** Client is buffering the response

**Solution:** 
- JavaScript: Use `response.body.getReader()`
- Python: Use `stream=True` parameter
- cURL: Use `-N` flag
- Go: Use `bufio.Scanner`

### Issue: Blank/incomplete output

**Cause:** Not reading the stream completely

**Solution:** Read until the stream is done/closed

### Issue: Cannot parse response

**Cause:** Expecting JSON/YAML format

**Solution:** Parse line-by-line text with prefixes

## Rollback Plan

If you need to temporarily maintain old behavior while migrating:

### Option 1: Keep old clients working with GET endpoint
The `GET /api/stacks/{name}` endpoint still returns YAML without streaming.

### Option 2: Add a query parameter check (server-side)
You could modify the server to check for a `?stream=false` parameter and use the old behavior.

## Support

- See `docs/CHUNKED_STREAMING.md` for complete documentation
- See `docs/STREAMING_QUICK_START.md` for quick examples
- Check example clients in `streaming-client.py` and `streaming-demo.html`

## Timeline

1. **Immediate:** Test with `curl -N` to verify functionality
2. **Day 1-2:** Update one client as a proof of concept
3. **Week 1:** Migrate all clients following this guide
4. **Week 2:** Deploy and monitor in production

## Checklist

- [ ] Read this migration guide
- [ ] Update HTTP client to handle streaming
- [ ] Implement progress callback/handler
- [ ] Update error handling for streamed errors
- [ ] Test with real endpoints
- [ ] Update UI to show real-time progress
- [ ] Deploy updated clients
- [ ] Monitor for issues

---

**Questions?** Review the examples in this guide and the reference implementations in `streaming-client.py` and `streaming-demo.html`.
