package pkg

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"github.com/olekukonko/tablewriter"
)

type Screen interface {
	Display()
	HandleInput(input string) bool
	NeedsRefresh() bool
}

type LandingScreen struct {
	display *DisplayManager
	docker  *DockerClient
	ticker  *time.Ticker
	done    chan bool
}

func NewLandingScreen(display *DisplayManager, docker *DockerClient) *LandingScreen {
	s := &LandingScreen{
		display: display,
		docker:  docker,
		done:    make(chan bool),
	}
	s.startPolling()
	return s
}

func (s *LandingScreen) startPolling() {
	s.ticker = time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.updateServices()
			case <-s.done:
				return
			}
		}
	}()
}

func (s *LandingScreen) stopPolling() {
	if s.ticker != nil {
		s.ticker.Stop()
		s.done <- true
	}
}

func (s *LandingScreen) updateServices() {
	if s.docker != nil {
		services, err := s.docker.GetServices()
		if err != nil {
			log.Printf("Error fetching services: %v", err)
		} else {
			s.docker.UpdateServices(services)
			s.display.UpdateServices(services)
			s.display.Display()
		}
	}
}

func (s *LandingScreen) Display() {
	if s.docker == nil {
		fmt.Println("Error: Docker client is not initialized")
		return
	}

	server := s.display.config.GetCurrentServer()
	fmt.Printf("Connected to %s (%s@%s)\n\n", server.Name, server.User, server.Host)

	withPorts, withoutPorts, err := s.docker.GetServicesByPortStatus()
	if err != nil {
		fmt.Printf("Error getting services: %v\n", err)
		return
	}

	// Sort services alphabetically
	sort.Slice(withPorts, func(i, j int) bool {
		return withPorts[i].Name < withPorts[j].Name
	})
	sort.Slice(withoutPorts, func(i, j int) bool {
		return withoutPorts[i].Name < withoutPorts[j].Name
	})

	if len(withPorts) == 0 && len(withoutPorts) == 0 {
		fmt.Println("No services found.")
	} else {
		s.display.displayServicesTable(withoutPorts, false)
		fmt.Println()
		s.display.displayServicesTable(withPorts, true)
	}

	fmt.Println("\nAvailable Actions:")
	fmt.Println("Enter service number to view details and manage conflicts")
	fmt.Println("[b]ack - Return to server list")
	fmt.Println("Press Ctrl+C to exit")
}

func (s *LandingScreen) HandleInput(input string) bool {
	if input == "b" || input == "back" {
		s.stopPolling()
		s.display.SetMode(ModeServerList)
		return true
	} else if idx := parseIndex(input); idx >= 0 && idx < len(s.display.currentServices) {
		s.stopPolling()
		s.display.selectedService = s.display.currentServices[idx]
		s.display.selectedIndex = idx
		s.display.SetMode(ModeServiceDetail)
		return true
	}
	return false
}

func (s *LandingScreen) NeedsRefresh() bool {
	return false
}

type ServerListScreen struct {
	display *DisplayManager
}

func NewServerListScreen(display *DisplayManager) *ServerListScreen {
	return &ServerListScreen{
		display: display,
	}
}

func (s *ServerListScreen) Display() {
	fmt.Println("Docker Remote Servers")
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"#", "Name", "Host", "User", "Status"})
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("─")
	table.SetColumnSeparator("│")
	table.SetRowSeparator("─")
	table.SetHeaderLine(true)
	table.SetBorder(true)

	for i, server := range s.display.config.Servers {
		status := []string{}
		if server.Name == s.display.config.CurrentServer {
			status = append(status, ColorGreen+"Current"+ColorReset)
		}
		if server.Name == s.display.config.DefaultServer {
			status = append(status, ColorYellow+"Default"+ColorReset)
		}
		statusStr := strings.Join(status, ", ")
		if statusStr == "" {
			statusStr = "-"
		}

		table.Append([]string{
			fmt.Sprintf("%d", i),
			server.Name,
			server.Host,
			server.User,
			statusStr,
		})
	}

	table.Render()

	fmt.Println("\nAvailable Actions:")
	fmt.Printf("Enter server number to connect (current: %s)\n", s.display.config.CurrentServer)
	fmt.Println("[a]dd     - Add a new server")
	fmt.Println("[r]emove  - Remove a server")
	fmt.Println("[d]efault - Set default server")
	fmt.Println("Press Ctrl+C to exit")
}

func (s *ServerListScreen) HandleInput(input string) bool {
	switch input {
	case "a":
		if err := s.display.handleAddServer(); err != nil {
			fmt.Printf("Failed to add server: %v\n", err)
			fmt.Println("Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}
		return true
	case "r":
		if err := s.display.handleRemoveServer(); err != nil {
			fmt.Printf("Failed to remove server: %v\n", err)
			fmt.Println("Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}
		return true
	case "d":
		if err := s.display.handleSetDefaultServer(); err != nil {
			fmt.Printf("Failed to set default server: %v\n", err)
			fmt.Println("Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}
		return true
	default:
		if idx, err := strconv.Atoi(input); err == nil && idx >= 0 && idx < len(s.display.config.Servers) {
			server := &s.display.config.Servers[idx]
			if err := s.display.config.SetCurrentServer(server.Name); err != nil {
				fmt.Printf("Failed to set current server: %v\n", err)
				fmt.Println("Press Enter to continue...")
				bufio.NewReader(os.Stdin).ReadBytes('\n')
				return true
			}
			sshClient, err := NewSSHClient(server.User, server.Host, server.KeyPath)
			if err != nil {
				fmt.Printf("Error creating SSH client: %v\n", err)
				fmt.Println("Press Enter to continue...")
				bufio.NewReader(os.Stdin).ReadBytes('\n')
				return true
			}
			dockerClient, err := NewDockerClient(sshClient)
			if err != nil {
				fmt.Printf("Error creating Docker client: %v\n", err)
				sshClient.Close()
				fmt.Println("Press Enter to continue...")
				bufio.NewReader(os.Stdin).ReadBytes('\n')
				return true
			}
			dockerClient.Start()
			s.display.SetDockerClient(dockerClient)
			s.display.SetMode(ModeOverview)
			return true
		}
	}
	return false
}

func (s *ServerListScreen) NeedsRefresh() bool {
	return false
}

type ServiceDetailScreen struct {
	display *DisplayManager
	docker  *DockerClient
	ticker  *time.Ticker
	done    chan bool
}

func NewServiceDetailScreen(display *DisplayManager, docker *DockerClient) *ServiceDetailScreen {
	s := &ServiceDetailScreen{
		display: display,
		docker:  docker,
		done:    make(chan bool),
	}
	s.startPolling()
	return s
}

func (s *ServiceDetailScreen) startPolling() {
	s.ticker = time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.updateService()
			case <-s.done:
				return
			}
		}
	}()
}

func (s *ServiceDetailScreen) stopPolling() {
	if s.ticker != nil {
		s.ticker.Stop()
		s.done <- true
	}
}

func (s *ServiceDetailScreen) updateService() {
	if s.docker != nil && s.display.selectedService != nil {
		services, err := s.docker.GetServices()
		if err != nil {
			log.Printf("Error fetching services: %v", err)
		} else {
			for _, service := range services {
				if service.Name == s.display.selectedService.Name {
					s.display.selectedService = service
					s.display.Display()
					break
				}
			}
		}
	}
}

func (s *ServiceDetailScreen) Display() {
	if s.display.selectedService == nil {
		return
	}

	fmt.Printf("Service Detail: %s\n\n", s.display.selectedService.Name)

	// Service info table
	infoTable := tablewriter.NewWriter(os.Stdout)
	infoTable.SetHeader([]string{"Property", "Value"})
	infoTable.SetAutoWrapText(false)
	infoTable.SetAutoFormatHeaders(true)
	infoTable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	infoTable.SetAlignment(tablewriter.ALIGN_LEFT)
	infoTable.SetCenterSeparator("─")
	infoTable.SetColumnSeparator("│")
	infoTable.SetRowSeparator("─")
	infoTable.SetHeaderLine(true)
	infoTable.SetBorder(true)

	infoTable.Append([]string{"Name", s.display.selectedService.Name})
	infoTable.Append([]string{"Health Status", s.display.colorizeHealth(s.display.selectedService.HealthStatus)})
	infoTable.Append([]string{"Forward Status", s.display.colorizeStatus(s.display.selectedService.ForwardStatus)})
	infoTable.Render()
	fmt.Println()

	// Ports table
	portsTable := tablewriter.NewWriter(os.Stdout)
	portsTable.SetHeader([]string{"#", "Remote Port", "Local Port", "Status", "Local Process"})
	portsTable.SetAutoWrapText(true)
	portsTable.SetAutoFormatHeaders(true)
	portsTable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	portsTable.SetAlignment(tablewriter.ALIGN_LEFT)
	portsTable.SetCenterSeparator("─")
	portsTable.SetColumnSeparator("│")
	portsTable.SetRowSeparator("─")
	portsTable.SetHeaderLine(true)
	portsTable.SetBorder(true)

	for i, port := range s.display.selectedService.ExposedPorts {
		status := ColorGreen + "Ready" + ColorReset
		localPort := s.docker.GetPortMapping(s.display.selectedService.Name, port)
		processInfo := "None"

		if isConflict := contains(s.display.selectedService.Conflicts, port); isConflict {
			status = ColorRed + "Conflict" + ColorReset
			if info := s.docker.GetLocalProcessForPort(port); info != nil {
				processInfo = fmt.Sprintf("%s\nPID: %s\nUser: %s\nCmd: %s", 
					info.Name, 
					info.PID,
					info.User,
					truncateString(info.Command, 50),
				)
			}
		} else if s.display.selectedService.ForwardStatus == StatusForwarded {
			status = ColorGreen + "Forwarded" + ColorReset
		}

		portsTable.Append([]string{
			fmt.Sprintf("%d", i),
			port,
			localPort,
			status,
			processInfo,
		})
	}

	portsTable.Render()

	fmt.Println("\nAvailable Actions:")
	fmt.Println("[b]ack     - Return to overview")
	fmt.Println("[#] remap  - Remap port by number (e.g., '0 8081' to change port 0's local port to 8081)")
	if len(s.display.selectedService.Conflicts) > 0 {
		fmt.Println("[#] kill   - Kill process using port by number (e.g., '0 kill')")
	}
}

func (s *ServiceDetailScreen) HandleInput(input string) bool {
	if input == "b" || input == "back" {
		s.stopPolling()
		s.display.SetMode(ModeOverview)
		s.display.selectedService = nil
		s.display.selectedIndex = -1
		return true
	}

	parts := strings.Fields(input)
	if len(parts) < 2 {
		return false
	}

	portIdx, err := strconv.Atoi(parts[0])
	if err != nil || portIdx < 0 || portIdx >= len(s.display.selectedService.ExposedPorts) {
		return false
	}

	port := s.display.selectedService.ExposedPorts[portIdx]
	cmd := parts[1]

	switch cmd {
	case "kill":
		s.display.handleKillProcess(port)
		return true
	case "remap":
		if len(parts) == 3 {
			s.display.handleRemapPort(port, parts[2])
			return true
		}
	}
	return false
}

func (s *ServiceDetailScreen) NeedsRefresh() bool {
	return false
}
