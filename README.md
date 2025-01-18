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

## Installation

### Prerequisites

- Go 1.23 or later
- SSH access to remote host(s)
- Docker installed on remote host(s)
- rsync installed locally and on remote host(s)

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
