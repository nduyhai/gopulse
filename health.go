package gopulse

// HealthChecker defines an interface for performing liveness and readiness checks for a system or service.
// Name provides the identifier or name of the health check.
// CheckLiveness checks if the system or service is alive and reachable.
// CheckReadiness checks if the system or service is ready to handle requests.
type HealthChecker interface {
	// Name returns the name of the health check.
	Name() string

	// CheckLiveness and CheckReadiness are called by the Pulse to perform the health checks.
	CheckLiveness() error

	// CheckReadiness and CheckReadiness are called by the Pulse to perform the health checks.
	CheckReadiness() error
}
