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

type Status string

const (
	StatusUp   Status = "UP"
	StatusDown Status = "DOWN"
)

type PulseResponse struct {
	Status  Status            `json:"status"`
	Details map[string]Status `json:"details,omitempty"`
}

func NewDownStatus(errors map[string]error) *PulseResponse {
	details := make(map[string]Status, len(errors))
	for k, _ := range errors {
		details[k] = StatusDown
	}
	return &PulseResponse{
		Status:  StatusDown,
		Details: details,
	}
}

func NewUpStatus() *PulseResponse {
	return &PulseResponse{
		Status: StatusUp,
	}
}
