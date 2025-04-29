package healths

// Noop implement default health checker
type Noop struct {
}

// Name the default name health checker
func (n Noop) Name() string {
	return "noop"
}

// CheckLiveness the default liveness error
func (n Noop) CheckLiveness() error {
	return nil
}

// CheckReadiness the default readiness error
func (n Noop) CheckReadiness() error {
	return nil
}
