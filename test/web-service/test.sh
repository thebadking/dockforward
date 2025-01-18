#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

# Cleanup function
cleanup() {
    echo -e "\n${GREEN}Cleaning up...${NC}"
    # Get remote host from env or use default
    REMOTE_HOST=${REMOTE_DOCKER_HOST:-"c1.local"}
    REMOTE_USER=${REMOTE_DOCKER_USER:-"c1user"}
    
    # Extract host without port
    HOST=$(echo $REMOTE_HOST | cut -d: -f1)
    
    # Get the context directory from the container name
    CONTEXT_DIR=$(docker ps --format '{{.Names}}' | grep -o 'docker-context-[a-f0-9]*' | head -1)
    if [ -n "$CONTEXT_DIR" ]; then
        # Run docker compose down directly on remote host
        ssh "$REMOTE_USER@$HOST" "cd /tmp/$CONTEXT_DIR && docker compose down"
    else
        echo "No running containers found to clean up"
    fi
}

# Set up cleanup trap for script exit
trap cleanup EXIT

echo "Starting test sequence..."

# Step 1: Build and start the service
echo -e "\n${GREEN}Step 1: Building and starting the service...${NC}"
docker compose up -d --build

# Wait for service to start
echo -e "\n${GREEN}Waiting for service to start (10s)...${NC}"
sleep 10

# Step 2: Check if service is running
echo -e "\n${GREEN}Step 2: Checking if service is running...${NC}"
if docker ps | grep -q "docker-context.*-web-1"; then
    echo "✓ Service is running"
else
    echo -e "${RED}✗ Service is not running${NC}"
    docker ps
    echo -e "\nContainer logs:"
    docker compose logs
    exit 1
fi

# Step 3: Test HTTP endpoint
echo -e "\n${GREEN}Step 3: Testing HTTP endpoint...${NC}"
response=$(curl -s http://localhost:3000)
if echo "$response" | grep -q "Hello from remote-docker test service"; then
    echo "✓ HTTP endpoint is accessible"
    echo "✓ Response: $response"
else
    echo -e "${RED}✗ HTTP endpoint test failed${NC}"
    echo "Response: $response"
    exit 1
fi

# Step 4: Check port forwarding in dockforward-monitor
echo -e "\n${GREEN}Step 4: Checking port forwarding status...${NC}"
if ps aux | grep -q "[r]-monitor"; then
    echo "✓ dockforward-monitor is running"
else
    echo -e "${RED}✗ dockforward-monitor is not running${NC}"
    exit 1
fi

echo -e "\n${GREEN}All tests passed successfully!${NC}"
