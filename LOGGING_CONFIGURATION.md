# Automatic Logging Configuration

## Overview
The system now automatically adds logging configuration to all services when generating effective Docker Compose files.

## Implementation

### Changes Made

1. **Added LoggingConfig struct** (stack.go, line ~160):
   ```go
   type LoggingConfig struct {
       Driver  string            `yaml:"driver"`
       Options map[string]string `yaml:"options,omitempty"`
   }
   ```

2. **Added Logging field to ComposeService struct** (stack.go, line ~158):
   ```go
   Logging *LoggingConfig `yaml:"logging,omitempty"`
   ```

3. **Added automatic logging configuration in enrichServices()** (stack.go, line ~2450):
   - If logging is not specified, adds default configuration:
     ```yaml
     logging:
       driver: "json-file"
       options:
         max-size: "10m"
         max-file: "3"
     ```
   - If logging driver is "json-file" but options are missing, fills in the defaults

## When Logging Configuration is Added

The logging configuration is automatically added when:

1. **PUT /api/stacks/{name}** - When updating a stack via the API
   - Reads the user-provided YAML
   - Enriches it with logging (and other enhancements)
   - Writes both .yml (original) and .effective.yml (enriched) files

2. **POST /api/enrich/{name}** - When enriching a stack YAML without saving
   - Returns the enriched YAML with logging configuration
   - Does not modify files or create secrets

3. **POST /api/stacks** - When creating a new stack
   - Creates both .yml and .effective.yml files
   - The .effective.yml includes logging configuration

## Default Configuration

All services in the effective YAML will have:

```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"  # Maximum log file size
    max-file: "3"     # Maximum number of log files to retain
```

This prevents unbounded log growth and helps manage disk space.

## Customization

Users can override the logging configuration in their base YAML:

```yaml
services:
  myservice:
    image: nginx
    logging:
      driver: "syslog"
      options:
        syslog-address: "tcp://192.168.0.1:514"
```

If only the driver is specified as "json-file" without options:
```yaml
services:
  myservice:
    image: nginx
    logging:
      driver: "json-file"
```

The system will automatically add the missing options:
```yaml
services:
  myservice:
    image: nginx
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

## Testing

To test the automatic logging configuration:

1. Update an existing stack via PUT /api/stacks/{name}
2. Check the .effective.yml file - all services should have logging configuration
3. Or use POST /api/enrich/{name} to preview the enriched YAML

## Benefits

- **Automatic log rotation**: Prevents logs from consuming all disk space
- **Consistent configuration**: All services get the same sensible defaults
- **Override capability**: Users can still customize if needed
- **Zero effort**: Works automatically for all stacks
