package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"crypto/sha256"
	"io"
	"io/ioutil"
	"github.com/spf13/cobra"
	dockforward "dockforward/pkg"
)

// getBinaryName returns the current binary name (docker or dockforward)
func getBinaryName() string {
	return filepath.Base(os.Args[0])
}

// getMonitorName returns the monitor binary name based on current binary name
func getMonitorName() string {
	if getBinaryName() == "dockforward" {
		return "dockforward-monitor"
	}
	return "docker-monitor"
}

var rootCmd = &cobra.Command{
	Use:                getBinaryName(), // Use executable name (docker or dockforward)
	Short:              "Execute Docker commands on a remote host",
	Run:                executeCommand,
	DisableFlagParsing: true, // Pass all flags through to docker
}

// calculateProjectHash generates a stable hash based on the absolute path
func calculateProjectHash(dir string) (string, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Create a stable hash of the absolute path
	hash := sha256.New()
	io.WriteString(hash, absPath)
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// createExcludeFile creates a temporary file containing exclusion patterns from .gitignore and .dockerignore
func createExcludeFile(dir string) (string, error) {
	tmpfile, err := ioutil.TempFile("", "exclude")
	if err != nil {
		return "", err
	}

	// Common patterns to always exclude
	commonPatterns := []string{
		".git/",
		".env",
		"node_modules/",
	}

	// Write common patterns
	for _, pattern := range commonPatterns {
		fmt.Fprintln(tmpfile, pattern)
	}

	// Read and append .gitignore if it exists
	if gitignore, err := ioutil.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
		tmpfile.Write(gitignore)
		fmt.Fprintln(tmpfile) // Add newline
	}

	// Read and append .dockerignore if it exists
	if dockerignore, err := ioutil.ReadFile(filepath.Join(dir, ".dockerignore")); err == nil {
		tmpfile.Write(dockerignore)
	}

	if err := tmpfile.Close(); err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

// syncDirectory synchronizes the local directory with remote
func syncDirectory(user, host, localDir, remoteDir string) error {
	// Create remote directory
	mkdirCmd := exec.Command("ssh", fmt.Sprintf("%s@%s", user, host), "mkdir", "-p", remoteDir)
	if err := mkdirCmd.Run(); err != nil {
		return fmt.Errorf("failed to create remote directory: %v", err)
	}

	// Create exclude file from .gitignore and .dockerignore
	excludeFile, err := createExcludeFile(localDir)
	if err != nil {
		return fmt.Errorf("failed to create exclude file: %v", err)
	}
	defer os.Remove(excludeFile)

	// Sync files using rsync
	rsyncArgs := []string{
		"-rlptDz",  // no -a, explicit flags instead
		"--chmod=Du=rwx,Dg=rx,Do=rx,Fu=rw,Fg=r,Fo=r", // explicit permissions
		"--delete", // delete extraneous files
		"--exclude-from", excludeFile, // use patterns from exclude file
		"-v",      // verbose output for debugging
		"-e", "ssh",
		fmt.Sprintf("%s/", localDir), // source with trailing slash
		fmt.Sprintf("%s@%s:%s/", user, host, remoteDir), // destination
	}

	fmt.Fprintf(os.Stderr, "Running rsync with args: %v\n", rsyncArgs)

	rsyncCmd := exec.Command("rsync", rsyncArgs...)
	output, err := rsyncCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// executeRemoteDocker executes a docker command on the remote host
func executeRemoteDocker(user, host string, args []string, remoteDir string, needsContext bool) error {
	// Build the remote command
	var remoteCmd string
	if needsContext {
		remoteCmd = fmt.Sprintf("cd %s && docker %s", remoteDir, strings.Join(args, " "))
	} else {
		remoteCmd = fmt.Sprintf("docker %s", strings.Join(args, " "))
	}
	
	// Execute the command over SSH with pseudo-terminal allocation
	cmd := exec.Command("ssh", "-t", fmt.Sprintf("%s@%s", user, host), remoteCmd)
	
	// Connect command's standard streams to our own
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Run the command
	return cmd.Run()
}

// cleanupOldContexts removes docker context directories older than 24 hours
func cleanupOldContexts(user, host string) error {
	// Find and remove old context directories (older than 24h)
	// Only look in our specific context directory path
	cleanupCmd := fmt.Sprintf(
		"cd /tmp && find . -maxdepth 1 -type d -name 'docker-context-*' -mtime +1 -exec rm -rf {} \\;",
	)
	
	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", user, host), cleanupCmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cleanup failed: %v\nOutput: %s", err, string(output))
	}
	
	return nil
}

// checkRemoteDocker verifies that the monitor is running
func checkRemoteDocker() error {
	// Check if monitor is in the process list
	monitorName := getMonitorName()
	cmd := exec.Command("pgrep", "-f", fmt.Sprintf("^%s$", monitorName))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s is not running. Please start it first with: %s", monitorName, monitorName)
	}
	return nil
}


func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func executeCommand(cmd *cobra.Command, args []string) {
	// Check if monitor is running
	if err := checkRemoteDocker(); err != nil {
		log.Fatal(err)
	}

	// Load configuration
	config, err := dockforward.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Get current server config
	server := config.GetCurrentServer()
	if server == nil {
		log.Fatalf("No server configured. Use '%s' to configure servers", getMonitorName())
	}

	// Extract host without port
	hostParts := strings.Split(server.Host, ":")
	host := hostParts[0]

	// Cleanup old context directories
	if err := cleanupOldContexts(server.User, host); err != nil {
		// Just log the error but continue
		log.Printf("Warning: Failed to cleanup old contexts: %v", err)
	}

	// Get current working directory
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Check if we need to sync the directory
	needsSync := true
	remoteDir := ""

	if len(os.Args) > 1 {
		// Commands that don't need context
		noContextCommands := map[string]bool{
			"ps":      true,
			"images":  true,
			"logs":    true,
			"exec":    true,
			"stop":    true,
			"start":   true,
			"rm":      true,
			"volume":  true,
			"network": true,
			"system":  true,
			"info":    true,
			"version": true,
		}
		if noContextCommands[os.Args[1]] || (len(os.Args) > 2 && os.Args[1] == "container") {
			needsSync = false
		}
	}

	// Only create and sync directory if needed
	if needsSync {
		// Calculate project hash for context directory name
		projectHash, err := calculateProjectHash(pwd)
		if err != nil {
			log.Fatalf("Failed to calculate project hash: %v", err)
		}

		// Create remote directory path using stable project hash
		remoteDir = fmt.Sprintf("/tmp/docker-context-%s", projectHash[:12])

		fmt.Fprintf(os.Stderr, "Syncing context to %s...\n", remoteDir)
		if err := syncDirectory(server.User, host, pwd, remoteDir); err != nil {
			log.Fatalf("Failed to sync directory: %v", err)
		}

		// Debug: List contents of remote directory after sync
		listCmd := exec.Command("ssh", fmt.Sprintf("%s@%s", server.User, host), 
			fmt.Sprintf("cd %s && ls -la", remoteDir))
		if output, err := listCmd.CombinedOutput(); err != nil {
			log.Printf("Warning: Failed to list remote directory: %v", err)
		} else {
			fmt.Fprintf(os.Stderr, "Remote directory contents:\n%s\n", output)
		}
	}

	// Execute docker command remotely
	args = os.Args[1:]
	if len(args) > 0 && args[0] == "compose" {
		// For docker compose commands, ensure we're using -f to specify the config file
		hasConfigFlag := false
		for i := 1; i < len(args); i++ {
			if args[i] == "-f" || args[i] == "--file" {
				hasConfigFlag = true
				break
			}
		}
		if !hasConfigFlag {
			// Insert -f flag with docker-compose.yml right after "compose" command
			newArgs := make([]string, 0, len(args)+2)
			newArgs = append(newArgs, args[0], "-f", "docker-compose.yml")
			newArgs = append(newArgs, args[1:]...)
			args = newArgs
		}
	}

	if err := executeRemoteDocker(server.User, host, args, remoteDir, needsSync); err != nil {
		os.Exit(1)
	}
}
