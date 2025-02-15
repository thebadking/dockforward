# dockforward

A tool for seamlessly working with Docker on remote hosts. It provides port forwarding and transparent context syncing, allowing you to use Docker commands locally while executing them on remote machines.

## Features

- Manage multiple remote Docker hosts
- Transparent context syncing between local and remote machines
- Automatic port forwarding for Docker containers
- Support for all Docker commands including build and compose
- Smart caching of build contexts
- Respects .gitignore and .dockerignore files
- Automatic cleanup of old context directories

## Why Use This?

### macOS-specific Benefits
- **Architecture Independence**: Resolves common arm64 architecture issues in Mac development by executing Docker commands on remote Linux hosts
- **Space Efficiency**: Eliminates the need for Docker Desktop's large VM footprint (64GB+) on Mac systems with limited storage
- **Native Performance**: By offloading Docker operations to remote Linux hosts, you get native Docker performance without the overhead of virtualization

<img width="757" alt="image" src="https://github.com/user-attachments/assets/f857d6fd-b2a7-4291-a823-d81ade0817f6" />

## Installation

### Prerequisites

- Go 1.23 or later
- SSH access to remote host(s)
- Docker installed on remote host(s)
- rsync installed locally and on remote host(s)

### Important: Pre-Installation Configuration

Note: Manual configuration is currently necessary as the server management interface has known issues with adding and removing servers.

Before using the tool, you'll need to manually edit the configuration file to ensure proper server management. You have two options:

1. For a single server setup, edit the default values in `main.go`'s `getSSHConfig` function:
```go
user = "your-username"
host = "your-server.com:22"
keyPath = "~/.ssh/your_key"
```

2. For multiple servers, create and edit `~/.config/dockforward/config.json`:
```bash
mkdir -p ~/.config/dockforward
```

### Manual Configuration

The configuration file is located at `~/.config/dockforward/config.json` and uses a JSON format. The structure includes:
- `servers`: Array of server configurations, each with:
  - `name`: Unique identifier for the server
  - `host`: Server address and SSH port
  - `user`: SSH username
  - `key_path`: Path to SSH private key
- `current_server`: Name of the active server
- `default_server`: Server to use on startup

Basic example:
```json
{
  "servers": [
    {
      "name": "default",
      "host": "remote-docker.local:22",
      "user": "dockeruser",
      "key_path": "~/.ssh/docker_rsa"
    }
  ],
  "current_server": "default",
  "default_server": "default"
}
```


Example - Multiple Environments:
```json
{
  "servers": [
    {
      "name": "dev",
      "host": "dev-docker.company.com:22",
      "user": "developer",
      "key_path": "~/.ssh/dev_rsa"
    },
    {
      "name": "staging",
      "host": "staging-docker.company.com:22",
      "user": "deployer",
      "key_path": "~/.ssh/staging_rsa"
    }
  ],
  "current_server": "dev",
  "default_server": "dev"
}
```

The configuration directory will be automatically created when you first run the tool. You can either use the monitor interface to configure servers or manually edit this JSON file. Make sure to maintain valid JSON syntax when editing manually.

If you only have one server, you edit manually edit the getSSHConfig function in the main.go file to reflect your server details.
### Installation Options

The tool can be installed in two ways:

1. **As 'dockforward' (default if Docker is installed locally)**
   ```bash
   make install
   source ~/.zshrc  # or ~/.bashrc for bash users
   ```
   This will install:
   - `dockforward` - The command-line tool for Docker operations
   - `dockforward-monitor` - The service monitor and configuration tool

2. **As 'docker' (if Docker is not installed locally)**
   ```bash
   make install
   source ~/.zshrc  # or ~/.bashrc for bash users
   ```
   When prompted, choose to install as 'docker'. This will install:
   - `docker` - The command-line tool for Docker operations
   - `docker-monitor` - The service monitor and configuration tool

To uninstall:
```bash
make uninstall
```

Server Setup:
expose the api port for the monitor service to work correctly

sudo nano /lib/systemd/system/docker.service
and modify this line:
#ExecStart=/usr/bin/dockerd -H fd:// --containerd=/run/containerd/containerd.sock
ExecStart=/usr/bin/dockerd -H fd:// -H tcp://0.0.0.0:51575 --containerd=/run/containerd/containerd.sock

## Usage

1. Start the monitor and configure your first remote server:
```bash
dockforward-monitor  # or docker-monitor if installed as 'docker'
```

2. In the monitor interface:
   - Press 'a' to add a server
   - Enter server details:
     - Name (e.g., "prod", "staging", "dev")
     - Host (e.g., "example.com:22")
     - User (SSH username)
     - SSH key path (default: ~/.ssh/id_rsa)

3. Use Docker commands as normal:
```bash
# If installed as 'dockforward':
dockforward ps
dockforward build -t myapp .
dockforward compose up -d

# If installed as 'docker':
docker ps
docker build -t myapp .
docker compose up -d
```

All commands are executed on the currently selected remote host, with automatic context syncing and port forwarding.

### Managing Remote Servers

The monitor interface allows you to:
- View all configured servers
- Switch between servers
- Add new servers
- Remove existing servers
- Set the default server

### Port Forwarding

The monitor automatically:
- Detects exposed ports in Docker containers
- Handles port conflicts with local processes
- Provides options to kill conflicting processes or remap ports
- Shows real-time status of port forwarding

## Development

### Running Tests

```bash
cd test/web-service
./test.sh
```

### Project Structure

- `cmd/docker/`: Docker command proxy implementation
- `pkg/`: Core functionality
  - `config.go`: Server configuration management
  - `docker.go`: Docker API client
  - `ssh.go`: SSH and port forwarding
  - `display.go`: Terminal UI

## License

MIT License - see [LICENSE](LICENSE) for details

## Project Status

### Completed Features
- ✅ Terminal UI with tables displaying remote services
- ✅ Automatic refresh of service information
- ✅ Automatic port forwarding detection for exposed Docker ports
- ✅ Port conflict detection and resolution interface
- ✅ Basic server switching functionality
- ✅ Docker/dockforward binary with command forwarding to remote host
- ✅ Remote shell output display in local terminal
- ✅ Automatic rsync of build context to remote host

### Work in Progress
- ⏳ Adding/removing servers through the interface (currently broken - use manual config)
- ⏳ Port conflict resolution functionality (interface done, port changing not implemented)
- ⏳ Improved error handling for SSH connection failures
- ⏳ Better feedback for context syncing operations
- ⏳ Configuration validation and auto-repair

Note: For the most up-to-date status of features and known issues, please check the source code and issue tracker.
