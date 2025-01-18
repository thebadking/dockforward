.PHONY: all clean build install check-env setup-shell

SHELL := /bin/bash
HOME_BIN := $(HOME)/bin
CONFIG_DIR := $(HOME)/.config/dockforward

all: build

build:
	@echo "Building dockforward..."
	@go build -o bin/dockforward-monitor
	@go build -o bin/dockforward cmd/docker/main.go

clean:
	rm -rf bin/

check-env:
	@echo "Checking environment..."
	@if [ ! -d "$(HOME_BIN)" ]; then \
		echo "Creating $(HOME_BIN)..."; \
		mkdir -p $(HOME_BIN); \
	fi
	@if [ ! -d "$(CONFIG_DIR)" ]; then \
		echo "Creating config directory..."; \
		mkdir -p $(CONFIG_DIR); \
	fi

setup-shell:
	@echo "Setting up shell configuration..."
	@SHELL_NAME=$$(basename $$SHELL); \
	if [ "$$SHELL_NAME" = "zsh" ]; then \
		PROFILE="$(HOME)/.zshrc"; \
	else \
		PROFILE="$(HOME)/.bashrc"; \
	fi; \
	if ! grep -q "PATH=.*$(HOME_BIN)" "$$PROFILE"; then \
		echo 'export PATH="$(HOME_BIN):$$PATH"' >> "$$PROFILE"; \
		echo "Added $(HOME_BIN) to PATH in $$PROFILE"; \
	fi

install: check-env build setup-shell
	@echo "Installing dockforward..."
	@if command -v docker > /dev/null; then \
		echo "Docker is already installed."; \
		echo "Installing as 'dockforward'..."; \
		cp bin/dockforward-monitor $(HOME_BIN)/; \
		cp bin/dockforward $(HOME_BIN)/; \
		echo "Installation complete. Run 'dockforward-monitor' to configure and start."; \
	else \
		read -p "Docker is not installed. Would you like to install dockforward as the 'docker' command? [Y/n] " choice; \
		case "$$choice" in \
			n|N) echo "Installing as 'dockforward'..."; \
			     cp bin/dockforward-monitor $(HOME_BIN)/; \
			     cp bin/dockforward $(HOME_BIN)/; \
			     echo "Installation complete. Run 'dockforward-monitor' to configure and start.";; \
			*) echo "Installing as 'docker'..."; \
			   cp bin/dockforward-monitor $(HOME_BIN)/docker-monitor; \
			   cp bin/dockforward $(HOME_BIN)/docker; \
			   echo "Installation complete. Run 'docker-monitor' to configure and start.";; \
		esac \
	fi
	@echo "Please run: source ~/.zshrc (or ~/.bashrc for bash users)"

uninstall:
	@echo "Uninstalling dockforward..."
	rm -f $(HOME_BIN)/dockforward-monitor
	rm -f $(HOME_BIN)/dockforward
	rm -f $(HOME_BIN)/docker-monitor
	rm -f $(HOME_BIN)/docker
	@echo "Uninstallation complete"

reinstall: uninstall install
