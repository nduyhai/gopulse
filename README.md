# GoPulse Health Aggregator

A powerful, event-driven health monitoring system for Go applications that supports prioritized health checks with liveness and readiness probes.

## Features

- **Priority-based Health Checks**: Define criticality levels for different components
- **Liveness & Readiness Probes**: Separate checks for service availability and readiness
- **Event-driven Updates**: Asynchronous health status updates
- **Configurable Expiry**: Automatic health check expiration
- **Graceful Shutdown**: Context-based cancellation support
- **Thread-safe**: Safe for concurrent use
- **Extensible**: Easy to add new health checkers
- **Auto-update**: Automatic periodic health checks with backoff
- **Configurable Backoff**: Exponential backoff for failed checks

## Installation

```bash
go get github.com/nduyhai/gopulse
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/nduyhai/gopulse"
)

func main() {
    // Create a new health aggregator with custom configuration
    ctx := context.Background()
    aggregator := gopulse.NewHealthAggregator(ctx,
        gopulse.WithAutoUpdate(10*time.Second),           // Check every 10 seconds
        gopulse.WithInitialDelay(2*time.Second),          // Wait 2 seconds before first check
        gopulse.WithBackoff(60*time.Second, 1.5),         // Max backoff 60s, factor 1.5
        gopulse.WithExpiryTime(30*time.Second),
        gopulse.WithUpdateBuffer(100),
        gopulse.WithStatusChangeCallback(func(name string, status *gopulse.HealthStatus) {
            log.Printf("Health status changed for %s: Liveness=%v, Readiness=%v", 
                name, status.Liveness, status.Readiness)
        }),
    )

    // Start the aggregator (auto-updates will begin)
    aggregator.Start()
    defer aggregator.Stop()

    // Register health checks
    dbChecker := &DBHealthChecker{}
    aggregator.RegisterHealthCheck(dbChecker, gopulse.PriorityCritical)

    // Manual health update (optional when auto-update is enabled)
    livenessErr := dbChecker.CheckLiveness()
    readinessErr := dbChecker.CheckReadiness()
    aggregator.UpdateHealth(dbChecker, livenessErr, readinessErr)

    // Check overall health
    liveness, readiness, livenessErrors, readinessErrors := aggregator.GetOverallHealth()
    log.Printf("Liveness: %v, Readiness: %v", liveness, readiness)
}
```

## Configuration Options

The health aggregator can be configured using functional options:

### Basic Configuration
- `WithExpiryTime(d time.Duration)`: Set the expiry time for health checks
- `WithUpdateBuffer(size int)`: Set the size of the update channel buffer
- `WithStatusChangeCallback(callback func(name string, status *HealthStatus))`: Set a callback for status changes

### Auto-update Configuration
- `WithAutoUpdate(interval time.Duration)`: Enable automatic health checking with specified interval
- `WithInitialDelay(delay time.Duration)`: Set delay before starting auto-updates
- `WithBackoff(maxBackoff time.Duration, factor float64)`: Configure backoff for failed checks

### Default Configuration
```go
ExpiryTime:        30 * time.Second
UpdateBuffer:      100
AutoUpdateEnabled: false
CheckInterval:     5 * time.Second
InitialDelay:      1 * time.Second
MaxBackoff:        30 * time.Second
BackoffFactor:     2.0
```

## Priority Levels

Health checks can be registered with different priority levels:

- `PriorityCritical`: Highest priority (e.g., database)
- `PriorityHigh`: High priority (e.g., cache)
- `PriorityMedium`: Medium priority (e.g., external services)
- `PriorityLow`: Lowest priority (e.g., non-essential services)

## Implementing Health Checkers

To create a custom health checker, implement the `HealthChecker` interface:

```go
type HealthChecker interface {
    Name() string
    CheckLiveness() error
    CheckReadiness() error
}
```

Example implementation:

```go
type DBHealthChecker struct {
    db *sql.DB
}

func (c *DBHealthChecker) Name() string {
    return "database"
}

func (c *DBHealthChecker) CheckLiveness() error {
    return c.db.Ping()
}

func (c *DBHealthChecker) CheckReadiness() error {
    // Check if database is ready to handle requests
    return c.db.CheckReadiness()
}
```

## API Reference

### HealthAggregator

```go
// NewHealthAggregator creates a new health aggregator instance
func NewHealthAggregator(ctx context.Context, opts ...Option) *HealthAggregator

// Start begins processing health updates and auto-updates if enabled
func (ha *HealthAggregator) Start()

// Stop gracefully shuts down the health aggregator
func (ha *HealthAggregator) Stop()

// RegisterHealthCheck adds a new health check to the aggregator
func (ha *HealthAggregator) RegisterHealthCheck(checker HealthChecker, priority Priority)

// UpdateHealth sends a health update to the aggregator
func (ha *HealthAggregator) UpdateHealth(checker HealthChecker, livenessErr, readinessErr error)

// GetLiveness returns the overall liveness status
func (ha *HealthAggregator) GetLiveness() (bool, map[string]error)

// GetReadiness returns the overall readiness status
func (ha *HealthAggregator) GetReadiness() (bool, map[string]error)

// GetOverallHealth returns both liveness and readiness status
func (ha *HealthAggregator) GetOverallHealth() (liveness, readiness bool, livenessErrors, readinessErrors map[string]error)
```

## Best Practices

1. **Priority Assignment**:
   - Assign `PriorityCritical` to essential services (database, main application)
   - Use `PriorityHigh` for important dependencies (cache, message queues)
   - Use `PriorityMedium` for external services
   - Use `PriorityLow` for non-essential services

2. **Health Check Implementation**:
   - Liveness checks should be lightweight and fast
   - Readiness checks can be more thorough
   - Include meaningful error messages
   - Consider implementing circuit breakers for external services

3. **Configuration**:
   - Set appropriate expiry times based on your service requirements
   - Configure buffer sizes based on expected update frequency
   - Use status change callbacks for monitoring and alerting
   - Enable auto-update for critical services
   - Configure backoff to prevent overwhelming failed services

4. **Auto-update Configuration**:
   - Set appropriate check intervals based on service criticality
   - Use initial delay to allow services to initialize
   - Configure backoff to handle temporary failures
   - Monitor backoff times for persistent issues

## License

MIT License - see LICENSE file for details
