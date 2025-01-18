package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"strconv"
	"syscall"
	"time"
	"io/ioutil"
	"github.com/spf13/cobra"
	dockforward "dockforward/pkg"
)

// getSSHConfig loads SSH configuration from config file with fallback defaults
func getSSHConfig() (user, host, keyPath string) {
	// Default values
	user = "c1user"
	host = "c1.local:22"
	keyPath = "~/.ssh/id_rsa"

	// Get config directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	configPath := filepath.Join(homeDir, ".config", "dockforward", "config")
	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return
	}

	// Parse config file
	lines := strings.Split(string(configData), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "REMOTE_DOCKER_USER":
			if value != "" {
				user = value
			}
		case "REMOTE_DOCKER_HOST":
			if value != "" {
				host = value
			}
		case "REMOTE_DOCKER_KEY_PATH":
			if value != "" {
				keyPath = value
			}
		}
	}

	return
}

// getConfigCommand returns a command to modify configuration
func getConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure connection settings",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("Failed to get home directory: %v", err)
			}

			configDir := filepath.Join(homeDir, ".config", "dockforward")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				log.Fatalf("Failed to create config directory: %v", err)
			}

			configPath := filepath.Join(configDir, "config")

			// Get current values
			user, host, keyPath := getSSHConfig()

			// Prompt for new values
			reader := bufio.NewReader(os.Stdin)

			fmt.Printf("Current host: %s\nNew host (press enter to keep current): ", host)
			if newHost, err := reader.ReadString('\n'); err == nil && strings.TrimSpace(newHost) != "" {
				host = strings.TrimSpace(newHost)
			}

			fmt.Printf("Current user: %s\nNew user (press enter to keep current): ", user)
			if newUser, err := reader.ReadString('\n'); err == nil && strings.TrimSpace(newUser) != "" {
				user = strings.TrimSpace(newUser)
			}

			fmt.Printf("Current SSH key path: %s\nNew path (press enter to keep current): ", keyPath)
			if newPath, err := reader.ReadString('\n'); err == nil && strings.TrimSpace(newPath) != "" {
				keyPath = strings.TrimSpace(newPath)
			}

			// Save new configuration
			config := fmt.Sprintf("REMOTE_DOCKER_HOST=%s\nREMOTE_DOCKER_USER=%s\nREMOTE_DOCKER_KEY_PATH=%s\n",
				host, user, keyPath)
			if err := ioutil.WriteFile(configPath, []byte(config), 0644); err != nil {
				log.Fatalf("Failed to save configuration: %v", err)
			}

			fmt.Println("Configuration updated successfully")
		},
	}
	return cmd
}

// getMonitorName returns the monitor binary name based on current binary name
func getMonitorName() string {
	if filepath.Base(os.Args[0]) == "docker" {
		return "docker-monitor"
	}
	return "dockforward-monitor"
}

func main() {
	rootCmd := &cobra.Command{
		Use:   getMonitorName(),
		Short: "Monitor and forward Docker ports from a remote host",
		Run: monitorCommand,
	}

	rootCmd.AddCommand(getConfigCommand())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func monitorCommand(cmd *cobra.Command, args []string) {
	// Load configuration
	config, err := dockforward.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create display manager
	display, err := dockforward.NewDisplayManager(config, nil)
	if err != nil {
		log.Fatalf("Error creating display manager: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		os.Exit(0)
	}()

	var dockerClient *dockforward.DockerClient
	var sshClient *dockforward.SSHClient

	// Attempt to connect to the default server
	if server := config.GetCurrentServer(); server != nil {
		sshClient, err = dockforward.NewSSHClient(server.User, server.Host, server.KeyPath)
		if err != nil {
			log.Printf("Error creating SSH client for default server: %v", err)
		} else {
			dockerClient, err = dockforward.NewDockerClient(sshClient)
			if err != nil {
				log.Printf("Error creating Docker client for default server: %v", err)
				sshClient.Close()
			} else {
				dockerClient.Start()
				display.SetDockerClient(dockerClient)
				display.SetMode(dockforward.ModeOverview)
				fmt.Println("Connected to default server. Starting service monitor...")
			}
		}
	}

	// Create channel for input
	inputChan := make(chan string)

	// Start input handling goroutine
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			input, err := reader.ReadString('\n')
			if err != nil {
				log.Printf("Error reading input: %v", err)
				continue
			}
			inputChan <- strings.TrimSpace(input)
		}
	}()

	// Display initial screen
	display.Display()

	// Main loop
	for {
		select {
		case input := <-inputChan:
			handled := display.HandleInput(input)
			if !handled {
				switch input {
				case "b", "back":
					if sshClient != nil {
						sshClient.Close()
						sshClient = nil
					}
					if dockerClient != nil {
						dockerClient = nil
					}
					display.SetDockerClient(nil)
					display.SetMode(dockforward.ModeServerList)
				default:
					if idx, err := strconv.Atoi(input); err == nil && idx >= 0 && idx < len(config.Servers) {
						server := &config.Servers[idx]
						if sshClient != nil {
							sshClient.Close()
						}
						sshClient, err = dockforward.NewSSHClient(server.User, server.Host, server.KeyPath)
						if err != nil {
							log.Printf("Error creating SSH client: %v", err)
							continue
						}
						dockerClient, err = dockforward.NewDockerClient(sshClient)
						if err != nil {
							log.Printf("Error creating Docker client: %v", err)
							sshClient.Close()
							continue
						}
						dockerClient.Start()
						display.SetDockerClient(dockerClient)
						display.SetMode(dockforward.ModeOverview)
						fmt.Printf("Connected to %s. Starting service monitor...\n", server.Name)
					}
				}
			}
			display.Display()

		case <-time.After(50 * time.Millisecond):
			// No input, continue to next iteration
		}
	}
}
