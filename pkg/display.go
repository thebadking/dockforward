package pkg

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"github.com/olekukonko/tablewriter"
)

// DisplayMode represents different view modes
type DisplayMode int

const (
	ModeServerList DisplayMode = iota
	ModeOverview
	ModeServiceDetail
)

// DisplayManager handles the rendering of service tables
type DisplayManager struct {
	docker          *DockerClient
	config          *Config
	selectedService *ServiceStatus
	selectedIndex   int
	currentScreen   Screen
	currentServices []*ServiceStatus // Store current sorted services with ports
	mode            DisplayMode
	mu              sync.RWMutex
}

func (d *DisplayManager) Mode() DisplayMode {
	return d.mode
}

// NewDisplayManager creates a new display manager
func NewDisplayManager(config *Config, dockerClient *DockerClient) (*DisplayManager, error) {
	dm := &DisplayManager{
		docker: dockerClient,
		config: config,
	}
	dm.SetMode(ModeServerList)
	return dm, nil
}

func (d *DisplayManager) SetDockerClient(client *DockerClient) {
	d.docker = client
	if d.currentScreen != nil {
		switch screen := d.currentScreen.(type) {
		case *LandingScreen:
			screen.docker = client
		case *ServiceDetailScreen:
			screen.docker = client
		}
	}
}

// UpdateServices updates the services in the display manager
func (d *DisplayManager) UpdateServices(services map[string]*ServiceStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var withPorts, withoutPorts []*ServiceStatus
	for _, service := range services {
		if len(service.ExposedPorts) > 0 {
			withPorts = append(withPorts, service)
		} else {
			withoutPorts = append(withoutPorts, service)
		}
	}

	// Sort services
	sort.Slice(withPorts, func(i, j int) bool {
		return withPorts[i].Name < withPorts[j].Name
	})
	sort.Slice(withoutPorts, func(i, j int) bool {
		return withoutPorts[i].Name < withoutPorts[j].Name
	})

	d.currentServices = withPorts
}

func (d *DisplayManager) SetMode(mode DisplayMode) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch mode {
	case ModeServerList:
		d.currentScreen = NewServerListScreen(d)
	case ModeOverview:
		d.currentScreen = NewLandingScreen(d, d.docker)
	case ModeServiceDetail:
		d.currentScreen = NewServiceDetailScreen(d, d.docker)
	}
}

func (d *DisplayManager) Display() {
	// Clear screen
	fmt.Print("\033[H\033[2J")

	if d.currentScreen != nil {
		d.currentScreen.Display()
	}
}

func (d *DisplayManager) HandleInput(input string) bool {
	if d.currentScreen != nil {
		return d.currentScreen.HandleInput(input)
	}
	return false
}

func (d *DisplayManager) UpdateDisplay() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.docker != nil && d.currentScreen.NeedsRefresh() {
		services, err := d.docker.GetServices()
		if err != nil {
			log.Printf("Error fetching services: %v", err)
		} else {
			d.docker.UpdateServices(services)
			d.UpdateServices(services)
		}
	}

	d.Display()
}

func (d *DisplayManager) handleKillProcess(port string) {
	if info := d.docker.GetLocalProcessForPort(port); info != nil {
		if err := d.docker.KillProcess(info.PID); err != nil {
			log.Printf("Failed to kill process: %v", err)
			return
		}
		if err := d.docker.RemapPort(d.selectedService, port, port); err != nil {
			log.Printf("Failed to update port status: %v", err)
			return
		}
		portMap := make(map[string]string)
		if err := d.docker.GetClient().ForwardPorts(d.selectedService, portMap); err != nil {
			log.Printf("Failed to forward port after killing process: %v", err)
			return
		}
	}
}

func (d *DisplayManager) handleRemapPort(port, newPort string) {
	localPorts, err := GetLocalInUsePorts()
	if err != nil {
		log.Printf("Failed to get local ports: %v", err)
		return
	}
	if IsPortInUse(newPort, localPorts) {
		log.Printf("New port %s is already in use", newPort)
		return
	}
	if err := d.docker.RemapPort(d.selectedService, port, newPort); err != nil {
		log.Printf("Failed to update port status: %v", err)
		return
	}
	portMap := make(map[string]string)
	portMap[port] = newPort
	if err := d.docker.GetClient().ForwardPorts(d.selectedService, portMap); err != nil {
		log.Printf("Failed to forward remapped port: %v", err)
		return
	}
}

// displayServicesTable renders a single table of services
func (d *DisplayManager) displayServicesTable(services []*ServiceStatus, showPorts bool) {
	table := tablewriter.NewWriter(os.Stdout)
	
	// Set headers
	headers := []string{}
	if showPorts {
		headers = append(headers, "#")
	}
	headers = append(headers, "Service", "Health")
	if showPorts {
		headers = append(headers, "Exposed Ports", "Forward Status", "Conflicts")
	}
	table.SetHeader(headers)

	// Set table style
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("─")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetHeaderLine(true)
	table.SetBorder(true)

	// Add rows
	for i, service := range services {
		row := []string{}
		if showPorts {
			row = append(row, fmt.Sprintf("%d", i))
		}
		
		row = append(row,
			service.Name,
			d.colorizeHealth(service.HealthStatus),
		)

		if showPorts {
			conflicts := "None"
			if len(service.Conflicts) > 0 {
				conflicts = ColorRed + strings.Join(service.Conflicts, ", ") + ColorReset
			}

			row = append(row,
				strings.Join(service.ExposedPorts, ", "),
				d.colorizeStatus(service.ForwardStatus),
				conflicts,
			)
		}

		table.Append(row)
	}

	table.Render()
}

// colorizeHealth returns health status with appropriate color
func (d *DisplayManager) colorizeHealth(health string) string {
	switch health {
	case HealthHealthy, HealthRunning:
		return ColorGreen + health + ColorReset
	case HealthUnhealthy, HealthDead:
		return ColorRed + health + ColorReset
	case HealthStarting, HealthRestarting:
		return ColorYellow + health + ColorReset
	default:
		return health
	}
}

// colorizeStatus returns forward status with appropriate color
func (d *DisplayManager) colorizeStatus(status string) string {
	switch status {
	case StatusForwarded:
		return ColorGreen + status + ColorReset
	case StatusConflict, StatusError:
		return ColorRed + status + ColorReset
	case StatusReady:
		return ColorGreen + status + ColorReset
	default:
		return status
	}
}

// Helper functions
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// parseIndex attempts to parse a string as a service index
func parseIndex(input string) int {
	if len(input) == 0 {
		return -1
	}
	
	// Convert input to integer
	var idx int
	_, err := fmt.Sscanf(input, "%d", &idx)
	if err != nil {
		return -1
	}
	
	return idx
}

// readInput reads a line of input with proper error handling
func readInput(reader *bufio.Reader, prompt string, required bool, defaultValue string) (string, error) {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %v", err)
	}

	value := strings.TrimSpace(input)
	if value == "" {
		if required {
			return "", fmt.Errorf("input cannot be empty")
		}
		return defaultValue, nil
	}

	return value, nil
}

// handleAddServer prompts for new server details
func (d *DisplayManager) handleAddServer() error {
	reader := bufio.NewReader(os.Stdin)

	name, err := readInput(reader, "\nEnter server name: ", true, "")
	if err != nil {
		return fmt.Errorf("error reading server name: %v", err)
	}

	host, err := readInput(reader, "Enter host (e.g., example.com:22): ", true, "")
	if err != nil {
		return fmt.Errorf("error reading host: %v", err)
	}

	user, err := readInput(reader, "Enter user: ", true, "")
	if err != nil {
		return fmt.Errorf("error reading user: %v", err)
	}

	keyPath, err := readInput(reader, "Enter SSH key path (default: ~/.ssh/id_rsa): ", false, "~/.ssh/id_rsa")
	if err != nil {
		return fmt.Errorf("error reading SSH key path: %v", err)
	}

	if err := d.config.AddServer(name, host, user, keyPath); err != nil {
		return fmt.Errorf("failed to add server: %v", err)
	}

	fmt.Printf("Server '%s' added successfully\n", name)
	return nil
}

// handleRemoveServer prompts for server index to remove
func (d *DisplayManager) handleRemoveServer() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("\nEnter server index to remove: ")
	indexStr, _ := reader.ReadString('\n')
	index, err := strconv.Atoi(strings.TrimSpace(indexStr))
	if err != nil {
		return fmt.Errorf("invalid input: please enter a valid number")
	}
	if index < 0 || index >= len(d.config.Servers) {
		return fmt.Errorf("invalid server index: must be between 0 and %d", len(d.config.Servers)-1)
	}

	serverName := d.config.Servers[index].Name
	fmt.Printf("Are you sure you want to remove server '%s'? (y/N): ", serverName)
	confirm, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
		return fmt.Errorf("server removal cancelled")
	}

	if err := d.config.RemoveServer(serverName); err != nil {
		return fmt.Errorf("failed to remove server: %v", err)
	}
	fmt.Printf("Server '%s' (index %d) removed successfully\n", serverName, index)
	return nil
}

// handleSetDefaultServer prompts for new default server index
func (d *DisplayManager) handleSetDefaultServer() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("\nEnter server index to set as default: ")
	indexStr, _ := reader.ReadString('\n')
	index, err := strconv.Atoi(strings.TrimSpace(indexStr))
	if err != nil {
		return fmt.Errorf("invalid input: please enter a valid number")
	}
	if index < 0 || index >= len(d.config.Servers) {
		return fmt.Errorf("invalid server index: must be between 0 and %d", len(d.config.Servers)-1)
	}

	serverName := d.config.Servers[index].Name
	if err := d.config.SetDefaultServer(serverName); err != nil {
		return fmt.Errorf("failed to set default server: %v", err)
	}
	fmt.Printf("Server '%s' (index %d) set as default successfully\n", serverName, index)
	return nil
}
