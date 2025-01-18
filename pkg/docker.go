package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"log"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DockerClient handles Docker API communication
type DockerClient struct {
	sshClient *SSHClient
	listener  net.Listener
	apiPort   int
	services  map[string]*ServiceStatus
	portMappings map[string]map[string]string // service name -> remote port -> local port
	mu        sync.RWMutex
}

// NewDockerClient creates a new Docker client that communicates via SSH
func NewDockerClient(sshClient *SSHClient) (*DockerClient, error) {
	// Create local listener for Docker API forwarding
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to create local listener: %v", err)
	}

	return &DockerClient{
		sshClient: sshClient,
		listener:  listener,
		apiPort:   listener.Addr().(*net.TCPAddr).Port,
		services:  make(map[string]*ServiceStatus),
		portMappings: make(map[string]map[string]string),
	}, nil
}

// Start initializes the Docker API connection
func (d *DockerClient) Start() {
	// Forward local port to Docker socket
	go func() {
		for {
			local, err := d.listener.Accept()
			if err != nil {
				log.Printf("Failed to accept connection: %v", err)
				return
			}

			remote, err := d.sshClient.GetClient().Dial("unix", "/var/run/docker.sock")
			if err != nil {
				log.Printf("Failed to connect to Docker socket: %v", err)
				local.Close()
				continue
			}

			go func() {
				defer local.Close()
				defer remote.Close()
				go func() { _, _ = io.Copy(local, remote) }()
				_, _ = io.Copy(remote, local)
			}()
		}
	}()

	log.Println("Docker API connection initialized")
}

// Close closes the Docker client
func (d *DockerClient) Close() error {
	return d.listener.Close()
}

// GetServices retrieves and processes Docker container information
func (d *DockerClient) GetServices() (map[string]*ServiceStatus, error) {
	// Query Docker API
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/containers/json", d.apiPort))
	if err != nil {
		return nil, fmt.Errorf("failed to query Docker API: %v", err)
	}
	defer resp.Body.Close()

	var containers []Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("failed to decode Docker API response: %v", err)
	}

	services := make(map[string]*ServiceStatus)

	for _, container := range containers {
		name := strings.TrimPrefix(container.Names[0], "/")
		ports := d.extractPorts(container.Ports)
		health := d.parseContainerState(container.State, container.Status)

		service := &ServiceStatus{
			Name:          name,
			ExposedPorts:  ports,
			HealthStatus:  health,
			ForwardStatus: StatusNotForwarded,
		}

		services[name] = service

		// Attempt to forward ports
		if err := d.forwardPorts(service); err != nil {
			log.Printf("Failed to forward ports for %s: %v", name, err)
		}
	}

	return services, nil
}

// forwardPorts attempts to forward the exposed ports for a service
func (d *DockerClient) forwardPorts(service *ServiceStatus) error {
	for _, port := range service.ExposedPorts {
		localPort := port // Use the same port number for local and remote
		err := d.sshClient.ForwardPort(localPort, port)
		if err != nil {
			service.ForwardStatus = StatusError
			return fmt.Errorf("failed to forward port %s: %v", port, err)
		}
	}
	service.ForwardStatus = StatusForwarded
	return nil
}

// UpdateServices updates the internal services map with the provided services
func (d *DockerClient) UpdateServices(services map[string]*ServiceStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.services = services
}

// extractPorts extracts port information from Docker API Port structs
func (d *DockerClient) extractPorts(ports []Port) []string {
	// Use a map to deduplicate ports
	portMap := make(map[string]bool)
	for _, port := range ports {
		if port.PublicPort != 0 {
			portMap[strconv.Itoa(port.PublicPort)] = true
		}
	}
	
	// Convert map keys to sorted slice
	var result []string
	for port := range portMap {
		result = append(result, port)
	}
	sort.Strings(result)
	return result
}

// parseContainerState parses container state from Docker API
func (d *DockerClient) parseContainerState(state, status string) string {
	switch state {
	case "running":
		if strings.Contains(status, "healthy") {
			return HealthHealthy
		} else if strings.Contains(status, "unhealthy") {
			return HealthUnhealthy
		}
		return HealthRunning
	case "created":
		return HealthCreated
	case "restarting":
		return HealthRestarting
	case "removing":
		return HealthRemoving
	case "paused":
		return HealthPaused
	case "exited":
		return HealthExited
	case "dead":
		return HealthDead
	default:
		return HealthUnknown
	}
}

// UpdateForwardingStatus updates the forwarding status of all services
func (d *DockerClient) UpdateForwardingStatus() error {
	localPorts, err := GetLocalInUsePorts()
	if err != nil {
		return fmt.Errorf("error getting local ports: %v", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, service := range d.services {
		// Reset conflicts and status
		conflicts := make(map[string]bool)
		service.ForwardStatus = StatusForwarded // Start with forwarded, will be changed if any conflicts found

		// Check each port
		for _, port := range service.ExposedPorts {
			if IsPortInUse(port, localPorts) {
				conflicts[port] = true
				service.ForwardStatus = StatusConflict // If any port conflicts, service status is conflict
			} else if service.ForwardStatus != StatusConflict {
				// Only update to Ready if we haven't found any conflicts
				service.ForwardStatus = StatusReady
			}
		}

		// Convert conflicts map to sorted slice
		service.Conflicts = []string{}
		for port := range conflicts {
			service.Conflicts = append(service.Conflicts, port)
		}
		sort.Strings(service.Conflicts)
	}

	return nil
}

// ProcessInfo represents information about a process using a port
type ProcessInfo struct {
	Name    string
	PID     string
	User    string
	Command string
}

// KillProcess kills a process by its PID
func (d *DockerClient) KillProcess(pid string) error {
	cmd := exec.Command("kill", pid)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill process: %v", err)
	}
	return nil
}

// RemapPort updates the port forwarding for a service
func (d *DockerClient) RemapPort(service *ServiceStatus, remotePort, localPort string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Initialize port mappings for service if not exists
	if _, exists := d.portMappings[service.Name]; !exists {
		d.portMappings[service.Name] = make(map[string]string)
	}

	// Store the new mapping
	d.portMappings[service.Name][remotePort] = localPort

	// Remove old port from conflicts if it exists
	if contains(service.Conflicts, remotePort) {
		service.Conflicts = removeString(service.Conflicts, remotePort)
	}

	// Update service status
	service.ForwardStatus = StatusReady

	return nil
}

// GetPortMapping returns the local port for a given service's remote port
func (d *DockerClient) GetPortMapping(serviceName, remotePort string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if mappings, exists := d.portMappings[serviceName]; exists {
		if localPort, exists := mappings[remotePort]; exists {
			return localPort
		}
	}
	return remotePort // Default to same port if no mapping exists
}

// Helper function to remove a string from a slice
func removeString(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// GetLocalProcessForPort returns detailed information about the local process using a port
func (d *DockerClient) GetLocalProcessForPort(port string) *ProcessInfo {
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%s", port), "-F", "pcun")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	lines := strings.Split(string(out), "\n")
	if len(lines) < 4 {
		return nil
	}

	info := &ProcessInfo{}
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			info.PID = line[1:]
		case 'c':
			info.Name = line[1:]
		case 'u':
			info.User = line[1:]
		case 'n':
			info.Command = line[1:]
		}
	}

	if info.PID == "" {
		return nil
	}

	// Get full command line
	if cmdBytes, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", info.PID)); err == nil {
		info.Command = strings.ReplaceAll(string(cmdBytes), "\x00", " ")
	}

	return info
}

// GetServicesByPortStatus returns services grouped by whether they have exposed ports
func (d *DockerClient) GetServicesByPortStatus() (withPorts, withoutPorts []*ServiceStatus, err error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d == nil {
		return nil, nil, fmt.Errorf("DockerClient is nil")
	}

	if d.services == nil {
		return nil, nil, fmt.Errorf("services map is nil")
	}

	for _, service := range d.services {
		if service != nil {
			if len(service.ExposedPorts) > 0 {
				withPorts = append(withPorts, service)
			} else {
				withoutPorts = append(withoutPorts, service)
			}
		}
	}

	return withPorts, withoutPorts, nil
}

// GetClient returns the SSH client
func (d *DockerClient) GetClient() *SSHClient {
	return d.sshClient
}
