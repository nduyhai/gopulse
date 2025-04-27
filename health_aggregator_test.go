package gopulse

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockHealthChecker implements the HealthChecker interface for testing
type mockHealthChecker struct {
	name         string
	livenessErr  error
	readinessErr error
	checkCount   int
	priority     Priority
}

func (m *mockHealthChecker) Name() string {
	return m.name
}

func (m *mockHealthChecker) CheckLiveness() error {
	m.checkCount++
	return m.livenessErr
}

func (m *mockHealthChecker) CheckReadiness() error {
	m.checkCount++
	return m.readinessErr
}

func TestNewHealthAggregator(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx)

	if ha == nil {
		t.Fatal("Expected non-nil HealthAggregator")
	}
}

func TestRegisterHealthCheck(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx)
	checker := &mockHealthChecker{name: "test", priority: PriorityCritical}

	ha.RegisterHealthCheck(checker, PriorityCritical)

	// Verify the checker was registered
	ha.mu.RLock()
	status, exists := ha.statuses[checker.name]
	ha.mu.RUnlock()

	if !exists {
		t.Fatal("Expected checker to be registered")
	}
	if status.Priority != PriorityCritical {
		t.Errorf("Expected priority %v, got %v", PriorityCritical, status.Priority)
	}
}

func TestUpdateHealth(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx)
	checker := &mockHealthChecker{name: "test", priority: PriorityCritical}

	ha.RegisterHealthCheck(checker, PriorityCritical)
	ha.Start()
	defer ha.Stop()

	// Test successful health update
	ha.UpdateHealth(checker, nil, nil)
	time.Sleep(100 * time.Millisecond) // Wait for update to be processed

	ha.mu.RLock()
	status := ha.statuses[checker.name]
	ha.mu.RUnlock()

	if !status.Liveness || !status.Readiness {
		t.Error("Expected health status to be true")
	}

	// Test failed health update
	err := errors.New("test error")
	ha.UpdateHealth(checker, err, err)
	time.Sleep(100 * time.Millisecond)

	ha.mu.RLock()
	status = ha.statuses[checker.name]
	ha.mu.RUnlock()

	if status.Liveness || status.Readiness {
		t.Error("Expected health status to be false")
	}
	if status.LivenessErr != err || status.ReadinessErr != err {
		t.Error("Expected error to be set")
	}
}

func TestGetLiveness(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx)
	criticalChecker := &mockHealthChecker{name: "critical", priority: PriorityCritical}
	lowChecker := &mockHealthChecker{name: "low", priority: PriorityLow}

	ha.RegisterHealthCheck(criticalChecker, PriorityCritical)
	ha.RegisterHealthCheck(lowChecker, PriorityLow)
	ha.Start()
	defer ha.Stop()

	// Test all healthy
	ha.UpdateHealth(criticalChecker, nil, nil)
	ha.UpdateHealth(lowChecker, nil, nil)
	time.Sleep(100 * time.Millisecond)

	healthy, errs := ha.GetLiveness()
	if !healthy || len(errs) > 0 {
		t.Error("Expected all checks to be healthy")
	}

	// Test critical failure
	ha.UpdateHealth(criticalChecker, errors.New("critical error"), nil)
	time.Sleep(100 * time.Millisecond)

	healthy, errs = ha.GetLiveness()
	if healthy || len(errs) == 0 {
		t.Error("Expected critical failure to be detected")
	}
}

func TestGetReadiness(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx)
	checker := &mockHealthChecker{name: "test", priority: PriorityCritical}

	ha.RegisterHealthCheck(checker, PriorityCritical)
	ha.Start()
	defer ha.Stop()

	// Test healthy
	ha.UpdateHealth(checker, nil, nil)
	time.Sleep(100 * time.Millisecond)

	ready, errs := ha.GetReadiness()
	if !ready || len(errs) > 0 {
		t.Error("Expected checker to be ready")
	}

	// Test not ready
	ha.UpdateHealth(checker, nil, errors.New("not ready"))
	time.Sleep(100 * time.Millisecond)

	ready, errs = ha.GetReadiness()
	if ready || len(errs) == 0 {
		t.Error("Expected checker to be not ready")
	}
}

func TestExpiry(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx, WithExpiryTime(100*time.Millisecond))
	checker := &mockHealthChecker{name: "test", priority: PriorityCritical}

	ha.RegisterHealthCheck(checker, PriorityCritical)
	ha.Start()
	defer ha.Stop()

	// Initial healthy state
	ha.UpdateHealth(checker, nil, nil)
	time.Sleep(50 * time.Millisecond)

	healthy, _ := ha.GetLiveness()
	if !healthy {
		t.Error("Expected checker to be healthy before expiry")
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	healthy, errs := ha.GetLiveness()
	if healthy || len(errs) == 0 {
		t.Error("Expected checker to be expired")
	}
}

func TestPriorityOrder(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx)
	critical := &mockHealthChecker{name: "critical", priority: PriorityCritical}
	high := &mockHealthChecker{name: "high", priority: PriorityHigh}
	medium := &mockHealthChecker{name: "medium", priority: PriorityMedium}
	low := &mockHealthChecker{name: "low", priority: PriorityLow}

	// Register in reverse order to test priority sorting
	ha.RegisterHealthCheck(low, PriorityLow)
	ha.RegisterHealthCheck(medium, PriorityMedium)
	ha.RegisterHealthCheck(high, PriorityHigh)
	ha.RegisterHealthCheck(critical, PriorityCritical)
	ha.Start()
	defer ha.Stop()

	// Make all checks fail
	ha.UpdateHealth(critical, errors.New("critical error"), nil)
	ha.UpdateHealth(high, errors.New("high error"), nil)
	ha.UpdateHealth(medium, errors.New("medium error"), nil)
	ha.UpdateHealth(low, errors.New("low error"), nil)
	time.Sleep(100 * time.Millisecond)

	// Should return critical error first
	_, errs := ha.GetLiveness()
	if len(errs) != 1 || errs["critical"] == nil {
		t.Error("Expected critical error to be returned first")
	}
}

func TestStatusChangeCallback(t *testing.T) {
	ctx := context.Background()
	var callbackCalled bool
	var lastStatus *HealthStatus

	ha := NewHealthAggregator(ctx,
		WithStatusChangeCallback(func(name string, status *HealthStatus) {
			callbackCalled = true
			lastStatus = status
		}),
	)

	checker := &mockHealthChecker{name: "test", priority: PriorityCritical}
	ha.RegisterHealthCheck(checker, PriorityCritical)
	ha.Start()
	defer ha.Stop()

	ha.UpdateHealth(checker, nil, nil)
	time.Sleep(100 * time.Millisecond)

	if !callbackCalled {
		t.Error("Expected status change callback to be called")
	}
	if lastStatus == nil || !lastStatus.Liveness || !lastStatus.Readiness {
		t.Error("Expected status to reflect healthy state")
	}
}

func TestGracefulShutdown(t *testing.T) {
	ctx := context.Background()
	ha := NewHealthAggregator(ctx)
	checker := &mockHealthChecker{name: "test", priority: PriorityCritical}

	ha.RegisterHealthCheck(checker, PriorityCritical)
	ha.Start()

	// Send some updates
	for i := 0; i < 10; i++ {
		ha.UpdateHealth(checker, nil, nil)
	}

	// Wait for updates to be processed
	time.Sleep(100 * time.Millisecond)

	// Stop the aggregator
	ha.Stop()

	// Verify we can still read the last state
	ha.mu.RLock()
	status, exists := ha.statuses[checker.name]
	ha.mu.RUnlock()

	if !exists {
		t.Fatal("Expected checker to exist in statuses")
	}

	if !status.Liveness || !status.Readiness {
		t.Error("Expected final state to be healthy")
	}
}
