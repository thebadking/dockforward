package pkg

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"strconv"
	"sync"
)

// SSHClient wraps the SSH connection and configuration
type SSHClient struct {
	client *ssh.Client
	config *ssh.ClientConfig
	user   string
	host   string
	mu     sync.Mutex
	ports  map[string]string // Track forwarded ports and their mappings
}

// NewSSHClient creates a new SSH client with the given credentials
func NewSSHClient(user, host, keyPath string) (*SSHClient, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("unable to get home directory: %v", err)
	}

	keyPath = fmt.Sprintf("%s%s", homeDir, keyPath[1:])

	key, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to remote host: %v", err)
	}

	return &SSHClient{
		client: client,
		config: config,
		user:   user,
		host:   host,
		ports:  make(map[string]string),
	}, nil
}

// Close closes the SSH connection
func (s *SSHClient) Close() error {
	return s.client.Close()
}

// GetClient returns the underlying SSH client
func (s *SSHClient) GetClient() *ssh.Client {
	return s.client
}

// ForwardPort forwards a single port using SSH with optional local port mapping
func (s *SSHClient) ForwardPort(remotePort, localPort string) error {
	if localPort == "" {
		localPort = remotePort
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if port is already mapped
	if mappedPort, exists := s.ports[remotePort]; exists {
		if mappedPort == localPort {
			return nil // Port already forwarded to the same local port
		}
		// Different local port, need to kill existing forward
		cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%s", remotePort))
		if out, err := cmd.Output(); err == nil {
			// Kill the existing SSH process
			pid := strings.TrimSpace(string(out))
			if pid != "" {
				exec.Command("kill", pid).Run()
			}
		}
		delete(s.ports, remotePort)
	}

	// Track the new mapping
	s.ports[remotePort] = localPort

	// Extract host from SSH host string (remove port)
	host := strings.Split(s.host, ":")[0]

	cmd := fmt.Sprintf("ssh -L %s:localhost:%s %s@%s -N", localPort, remotePort, s.user, host)
	cmdArgs := strings.Split(cmd, " ")

	// Run the port forwarding command in a goroutine
	go func() {
		for {
			if err := exec.Command(cmdArgs[0], cmdArgs[1:]...).Run(); err != nil {
				log.Printf("Port forwarding for %s -> %s failed, retrying: %v", remotePort, localPort, err)
				s.mu.Lock()
				delete(s.ports, remotePort)
				s.mu.Unlock()
				return
			}
		}
	}()

	return nil
}

// ForwardPorts forwards multiple ports for a service with optional port mapping
func (s *SSHClient) ForwardPorts(service *ServiceStatus, portMap map[string]string) error {
	if portMap == nil {
		portMap = make(map[string]string)
	}

	for _, remotePort := range service.ExposedPorts {
		// Handle port ranges
		if strings.Contains(remotePort, "-") {
			portRange := strings.Split(remotePort, "-")
			if len(portRange) != 2 {
				continue
			}

			start, err := strconv.Atoi(portRange[0])
			if err != nil {
				continue
			}
			end, err := strconv.Atoi(portRange[1])
			if err != nil {
				continue
			}

			for i := start; i <= end; i++ {
				portStr := fmt.Sprintf("%d", i)
				localPort := portMap[portStr]
				if err := s.ForwardPort(portStr, localPort); err != nil {
					return fmt.Errorf("error forwarding port %s -> %s: %v", portStr, localPort, err)
				}
			}
		} else {
			localPort := portMap[remotePort]
			if err := s.ForwardPort(remotePort, localPort); err != nil {
				return fmt.Errorf("error forwarding port %s -> %s: %v", remotePort, localPort, err)
			}
		}
	}
	return nil
}

// GetLocalInUsePorts returns a list of ports in use on the local machine
func GetLocalInUsePorts() ([]string, error) {
	cmd := exec.Command("lsof", "-i", "-n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-use ports: %v", err)
	}

	return parseLocalPorts(string(out)), nil
}

// parseLocalPorts parses lsof output to extract listening ports
func parseLocalPorts(lsofOutput string) []string {
	lines := strings.Split(lsofOutput, "\n")
	var ports []string

	for _, line := range lines {
		if strings.Contains(line, "LISTEN") {
			parts := strings.Fields(line)
			if len(parts) > 8 {
				address := parts[8]
				parts = strings.Split(address, ":")
				if len(parts) > 1 {
					ports = append(ports, parts[1])
				}
			}
		}
	}
	return ports
}

// IsPortInUse checks if a port is in use locally
func IsPortInUse(port string, localPorts []string) bool {
	for _, p := range localPorts {
		if p == port {
			return true
		}
	}
	return false
}
