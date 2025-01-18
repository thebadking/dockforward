package pkg

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

type ServerConfig struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	User    string `json:"user"`
	KeyPath string `json:"key_path"`
}

type Config struct {
	Servers        []ServerConfig `json:"servers"`
	CurrentServer  string         `json:"current_server"`
	DefaultServer  string         `json:"default_server"`
}

func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %v", err)
	}
	return filepath.Join(homeDir, ".config", "dockforward"), nil
}

func LoadConfig() (*Config, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return createDefaultConfig()
	}

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	// Validate and clean up the configuration
	config.validateAndCleanup()

	// If no valid servers remain, create a default configuration
	if len(config.Servers) == 0 {
		return createDefaultConfig()
	}

	return &config, config.Save()
}

func createDefaultConfig() (*Config, error) {
	config := &Config{
		Servers: []ServerConfig{
			{
				Name:    "default",
				Host:    "c1.local:22",
				User:    "c1user",
				KeyPath: "~/.ssh/id_rsa",
			},
		},
		DefaultServer: "default",
		CurrentServer: "default",
	}
	return config, config.Save()
}

func (c *Config) validateAndCleanup() {
	validServers := []ServerConfig{}
	for _, server := range c.Servers {
		if server.isValid() {
			validServers = append(validServers, server)
		}
	}
	c.Servers = validServers

	// Ensure DefaultServer and CurrentServer are valid
	if !c.isServerNameValid(c.DefaultServer) {
		c.DefaultServer = ""
	}
	if !c.isServerNameValid(c.CurrentServer) {
		c.CurrentServer = c.DefaultServer
	}

	// If no default server, set it to the first valid server
	if c.DefaultServer == "" && len(c.Servers) > 0 {
		c.DefaultServer = c.Servers[0].Name
		c.CurrentServer = c.Servers[0].Name
	}
}

func (s *ServerConfig) isValid() bool {
	// Add more validation if needed
	return s.Name != "" && s.Host != "" && s.User != "" && s.KeyPath != ""
}

func (c *Config) isServerNameValid(name string) bool {
	for _, server := range c.Servers {
		if server.Name == name {
			return true
		}
	}
	return false
}

func (c *Config) Save() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	configPath := filepath.Join(configDir, "config.json")
	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %v", err)
	}

	return nil
}

func (c *Config) GetCurrentServer() *ServerConfig {
	for _, server := range c.Servers {
		if server.Name == c.CurrentServer {
			return &server
		}
	}
	// Fallback to default server
	for _, server := range c.Servers {
		if server.Name == c.DefaultServer {
			return &server
		}
	}
	// Fallback to first server
	if len(c.Servers) > 0 {
		return &c.Servers[0]
	}
	return nil
}

func (c *Config) AddServer(name, host, user, keyPath string) error {
	// Check if server already exists
	for _, server := range c.Servers {
		if server.Name == name {
			return fmt.Errorf("server %q already exists", name)
		}
	}

	c.Servers = append(c.Servers, ServerConfig{
		Name:    name,
		Host:    host,
		User:    user,
		KeyPath: keyPath,
	})

	// If this is the first server, make it the default and current
	if len(c.Servers) == 1 {
		c.DefaultServer = name
		c.CurrentServer = name
	}

	return c.Save()
}

func (c *Config) RemoveServer(name string) error {
	if name == c.DefaultServer {
		return fmt.Errorf("cannot remove default server")
	}

	for i, server := range c.Servers {
		if server.Name == name {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			if c.CurrentServer == name {
				c.CurrentServer = c.DefaultServer
			}
			return c.Save()
		}
	}

	return fmt.Errorf("server %q not found", name)
}

func (c *Config) SetCurrentServer(name string) error {
	for _, server := range c.Servers {
		if server.Name == name {
			c.CurrentServer = name
			return c.Save()
		}
	}
	return fmt.Errorf("server %q not found", name)
}

func (c *Config) SetDefaultServer(name string) error {
	for _, server := range c.Servers {
		if server.Name == name {
			c.DefaultServer = name
			return c.Save()
		}
	}
	return fmt.Errorf("server %q not found", name)
}
