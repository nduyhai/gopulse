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
	// Auto update configuration
	AutoUpdateEnabled bool
	CheckInterval     time.Duration
	InitialDelay      time.Duration
	MaxBackoff        time.Duration
	BackoffFactor     float64
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

// WithAutoUpdate enables automatic health checking with the specified interval
func WithAutoUpdate(interval time.Duration) Option {
	return func(c *Config) {
		c.AutoUpdateEnabled = true
		c.CheckInterval = interval
	}
}

// WithInitialDelay sets the initial delay before starting auto-updates
func WithInitialDelay(delay time.Duration) Option {
	return func(c *Config) {
		c.InitialDelay = delay
	}
}

// WithBackoff sets the backoff configuration for failed checks
func WithBackoff(maxBackoff time.Duration, factor float64) Option {
	return func(c *Config) {
		c.MaxBackoff = maxBackoff
		c.BackoffFactor = factor
	}
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		ExpiryTime:        30 * time.Second,
		UpdateBuffer:      100,
		OnStatusChange:    nil,
		AutoUpdateEnabled: false,
		CheckInterval:     5 * time.Second,
		InitialDelay:      1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffFactor:     2.0,
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
	// Auto update state
	checkers         map[string]HealthChecker
	backoffTimes     map[string]time.Duration
	lastCheckAttempt map[string]time.Time
}

// NewHealthAggregator creates a new health aggregator instance with optional configuration
func NewHealthAggregator(ctx context.Context, opts ...Option) *HealthAggregator {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	ctx, cancel := context.WithCancel(ctx)
	return &HealthAggregator{
		statuses:         make(map[string]*HealthStatus),
		config:           config,
		ctx:              ctx,
		cancel:           cancel,
		updateChannel:    make(chan *HealthStatus, config.UpdateBuffer),
		checkers:         make(map[string]HealthChecker),
		backoffTimes:     make(map[string]time.Duration),
		lastCheckAttempt: make(map[string]time.Time),
	}
}

// Start begins processing health updates and auto-updates if enabled
func (ha *HealthAggregator) Start() {
	go ha.processUpdates()
	if ha.config.AutoUpdateEnabled {
		go ha.autoUpdate()
	}
}

// Stop gracefully shuts down the health aggregator
func (ha *HealthAggregator) Stop() {
	ha.cancel()
}

// RegisterHealthCheck adds a new health check to the aggregator
func (ha *HealthAggregator) RegisterHealthCheck(checker HealthChecker, priority Priority) {
	ha.mu.Lock()
	defer ha.mu.Unlock()

	name := checker.Name()
	ha.checkers[name] = checker
	ha.statuses[name] = &HealthStatus{
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

// autoUpdate performs automatic health checks for registered checkers
func (ha *HealthAggregator) autoUpdate() {
	// Initial delay
	select {
	case <-ha.ctx.Done():
		return
	case <-time.After(ha.config.InitialDelay):
		// Perform initial checks immediately after delay
		ha.mu.RLock()
		checkers := make([]HealthChecker, 0, len(ha.checkers))
		for _, checker := range ha.checkers {
			checkers = append(checkers, checker)
		}
		ha.mu.RUnlock()

		for _, checker := range checkers {
			ha.checkHealth(checker)
		}
	}

	ticker := time.NewTicker(ha.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ha.ctx.Done():
			return
		case <-ticker.C:
			ha.mu.RLock()
			checkers := make([]HealthChecker, 0, len(ha.checkers))
			for _, checker := range ha.checkers {
				checkers = append(checkers, checker)
			}
			ha.mu.RUnlock()

			for _, checker := range checkers {
				ha.checkHealth(checker)
			}
		}
	}
}

// checkHealth performs a health check with backoff
func (ha *HealthAggregator) checkHealth(checker HealthChecker) {
	name := checker.Name()
	now := time.Now()

	// Check if we should skip this check due to backoff
	ha.mu.RLock()
	backoff := ha.backoffTimes[name]
	lastAttempt, exists := ha.lastCheckAttempt[name]
	ha.mu.RUnlock()

	if backoff > 0 && exists {
		// Calculate time since last check attempt
		timeSinceLastAttempt := now.Sub(lastAttempt)
		if timeSinceLastAttempt < backoff {
			// Skip this check as we're still in backoff period
			return
		}
	}

	// Update last check attempt time before performing the check
	ha.mu.Lock()
	ha.lastCheckAttempt[name] = now
	ha.mu.Unlock()

	// Perform health checks
	livenessErr := checker.CheckLiveness()
	readinessErr := checker.CheckReadiness()

	// Update backoff time based on check results
	ha.mu.Lock()
	if livenessErr != nil || readinessErr != nil {
		// Increase backoff time
		if backoff == 0 {
			// Start with check interval as initial backoff
			backoff = ha.config.CheckInterval / 2
		} else {
			// Exponential backoff
			backoff = time.Duration(float64(backoff) * ha.config.BackoffFactor)
		}
		// Cap at max backoff
		if backoff > ha.config.MaxBackoff {
			backoff = ha.config.MaxBackoff
		}
		ha.backoffTimes[name] = backoff
	} else {
		// Reset backoff on success
		ha.backoffTimes[name] = 0
	}
	ha.mu.Unlock()

	// Send update
	ha.UpdateHealth(checker, livenessErr, readinessErr)
}

// ErrHealthCheckExpired is returned when a health check has not been updated within the expiry time
var ErrHealthCheckExpired = errors.New("health check has expired")
