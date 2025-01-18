package pkg

// Docker API types
type Container struct {
	ID     string
	Names  []string
	State  string
	Status string
	Ports  []Port
}

type Port struct {
	IP          string
	PrivatePort int
	PublicPort  int
	Type        string
}

// ServiceStatus represents the current state of a Docker service
type ServiceStatus struct {
	Name           string
	ExposedPorts   []string
	HealthStatus   string
	ForwardStatus  string
	LocalPorts     []string
	Conflicts      []string
}


// Forward status constants
const (
	StatusNotForwarded = "Not forwarded"
	StatusForwarded    = "Forwarded"
	StatusReady       = "Ready"
	StatusError       = "Error"
	StatusConflict    = "Conflict"
)

// Health status constants
const (
	HealthHealthy    = "Healthy"
	HealthUnhealthy  = "Unhealthy"
	HealthStarting   = "Starting"
	HealthRunning    = "Running"
	HealthRestarting = "Restarting"
	HealthCreated    = "Created"
	HealthRemoving   = "Removing"
	HealthPaused     = "Paused"
	HealthExited     = "Exited"
	HealthDead       = "Dead"
	HealthUnknown    = "Unknown"
)
