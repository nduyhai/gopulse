package gopulse

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Priority represents the importance level of a health check
type Priority int

const (
	PriorityCritical Priority = iota
	PriorityHigh
	PriorityMedium
	PriorityLow
)

// HealthStatus represents the current state of a health check
type HealthStatus struct {
	Checker      HealthChecker
	Priority     Priority
	Liveness     bool
	Readiness    bool
	LastUpdate   time.Time
	LivenessErr  error
	ReadinessErr error
}

// Config holds the configuration for the HealthAggregator
type Config struct {
	ExpiryTime     time.Duration
	UpdateBuffer   int
	OnStatusChange func(name string, status *HealthStatus)
}

// Option is a function that configures the HealthAggregator
type Option func(*Config)

// WithExpiryTime sets the expiry time for health checks
func WithExpiryTime(d time.Duration) Option {
	return func(c *Config) {
		c.ExpiryTime = d
	}
}

// WithUpdateBuffer sets the size of the update channel buffer
func WithUpdateBuffer(size int) Option {
	return func(c *Config) {
		c.UpdateBuffer = size
	}
}

// WithStatusChangeCallback sets a callback function for status changes
func WithStatusChangeCallback(callback func(name string, status *HealthStatus)) Option {
	return func(c *Config) {
		c.OnStatusChange = callback
	}
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		ExpiryTime:     30 * time.Second,
		UpdateBuffer:   100,
		OnStatusChange: nil,
	}
}

// HealthAggregator manages and aggregates health checks
type HealthAggregator struct {
	mu            sync.RWMutex
	statuses      map[string]*HealthStatus
	config        *Config
	ctx           context.Context
	cancel        context.CancelFunc
	updateChannel chan *HealthStatus
}

// NewHealthAggregator creates a new health aggregator instance with optional configuration
func NewHealthAggregator(ctx context.Context, opts ...Option) *HealthAggregator {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	ctx, cancel := context.WithCancel(ctx)
	return &HealthAggregator{
		statuses:      make(map[string]*HealthStatus),
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		updateChannel: make(chan *HealthStatus, config.UpdateBuffer),
	}
}

// Start begins processing health updates
func (ha *HealthAggregator) Start() {
	go ha.processUpdates()
}

// Stop gracefully shuts down the health aggregator
func (ha *HealthAggregator) Stop() {
	ha.cancel()
}

// RegisterHealthCheck adds a new health check to the aggregator
func (ha *HealthAggregator) RegisterHealthCheck(checker HealthChecker, priority Priority) {
	ha.mu.Lock()
	defer ha.mu.Unlock()

	ha.statuses[checker.Name()] = &HealthStatus{
		Checker:    checker,
		Priority:   priority,
		LastUpdate: time.Now(),
	}
}

// UpdateHealth sends a health update to the aggregator
func (ha *HealthAggregator) UpdateHealth(checker HealthChecker, livenessErr, readinessErr error) {
	ha.mu.RLock()
	status, exists := ha.statuses[checker.Name()]
	ha.mu.RUnlock()

	if !exists {
		return
	}

	ha.updateChannel <- &HealthStatus{
		Checker:      checker,
		Priority:     status.Priority,
		Liveness:     livenessErr == nil,
		Readiness:    readinessErr == nil,
		LastUpdate:   time.Now(),
		LivenessErr:  livenessErr,
		ReadinessErr: readinessErr,
	}
}

// GetLiveness returns the overall liveness status based on priorities
func (ha *HealthAggregator) GetLiveness() (bool, map[string]error) {
	ha.mu.RLock()
	defer ha.mu.RUnlock()

	errs := make(map[string]error)
	now := time.Now()

	// Check each priority level in order
	for _, priority := range []Priority{PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow} {
		for name, status := range ha.statuses {
			if status.Priority != priority {
				continue
			}

			// Check if the status has expired
			if now.Sub(status.LastUpdate) > ha.config.ExpiryTime {
				errs[name] = ErrHealthCheckExpired
				return false, errs
			}

			if !status.Liveness {
				errs[name] = status.LivenessErr
				return false, errs
			}
		}
	}

	return true, nil
}

// GetReadiness returns the overall readiness status based on priorities
func (ha *HealthAggregator) GetReadiness() (bool, map[string]error) {
	ha.mu.RLock()
	defer ha.mu.RUnlock()

	errs := make(map[string]error)
	now := time.Now()

	// Check each priority level in order
	for _, priority := range []Priority{PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow} {
		for name, status := range ha.statuses {
			if status.Priority != priority {
				continue
			}

			// Check if the status has expired
			if now.Sub(status.LastUpdate) > ha.config.ExpiryTime {
				errs[name] = ErrHealthCheckExpired
				return false, errs
			}

			if !status.Readiness {
				errs[name] = status.ReadinessErr
				return false, errs
			}
		}
	}

	return true, nil
}

// GetOverallHealth returns both liveness and readiness status
func (ha *HealthAggregator) GetOverallHealth() (liveness, readiness bool, livenessErrors, readinessErrors map[string]error) {
	liveness, livenessErrors = ha.GetLiveness()
	readiness, readinessErrors = ha.GetReadiness()
	return
}

// processUpdates handles incoming health updates
func (ha *HealthAggregator) processUpdates() {
	for {
		select {
		case <-ha.ctx.Done():
			return
		case status := <-ha.updateChannel:
			ha.mu.Lock()
			name := status.Checker.Name()
			ha.statuses[name] = status
			ha.mu.Unlock()

			// Call status change callback if configured
			if ha.config.OnStatusChange != nil {
				ha.config.OnStatusChange(name, status)
			}
		}
	}
}

// ErrHealthCheckExpired is returned when a health check has not been updated within the expiry time
var ErrHealthCheckExpired = errors.New("health check has expired")
