#!/bin/bash
#
# cleanup-service-integration.sh
#
# Removes devproxy system service files that were installed by previous versions.
# Run this script if you previously used `devproxy setup --service` or manually
# installed devproxy as a launchd/systemd service.
#
# Usage: sudo ./cleanup-service-integration.sh
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root (use sudo)"
    exit 1
fi

echo "Devproxy Service Cleanup Script"
echo "================================"
echo ""

# Detect OS
case "$(uname -s)" in
    Darwin)
        info "Detected macOS"
        
        PLIST_PATH="/Library/LaunchDaemons/com.devproxy.daemon.plist"
        
        if [[ -f "$PLIST_PATH" ]]; then
            info "Found launchd plist at $PLIST_PATH"
            
            # Unload the service if loaded
            if launchctl list | grep -q "com.devproxy.daemon"; then
                info "Unloading service..."
                launchctl unload "$PLIST_PATH" 2>/dev/null || true
            fi
            
            # Remove the plist file
            info "Removing plist file..."
            rm -f "$PLIST_PATH"
            
            info "macOS service removed successfully"
        else
            info "No launchd plist found at $PLIST_PATH"
        fi
        ;;
        
    Linux)
        info "Detected Linux"
        
        SYSTEMD_PATH="/etc/systemd/system/devproxy.service"
        
        if [[ -f "$SYSTEMD_PATH" ]]; then
            info "Found systemd unit at $SYSTEMD_PATH"
            
            # Stop the service if running
            if systemctl is-active --quiet devproxy 2>/dev/null; then
                info "Stopping service..."
                systemctl stop devproxy
            fi
            
            # Disable the service
            if systemctl is-enabled --quiet devproxy 2>/dev/null; then
                info "Disabling service..."
                systemctl disable devproxy
            fi
            
            # Remove the unit file
            info "Removing unit file..."
            rm -f "$SYSTEMD_PATH"
            
            # Reload systemd
            info "Reloading systemd daemon..."
            systemctl daemon-reload
            
            info "Linux service removed successfully"
        else
            info "No systemd unit found at $SYSTEMD_PATH"
        fi
        ;;
        
    *)
        error "Unsupported operating system: $(uname -s)"
        exit 1
        ;;
esac

echo ""
info "Cleanup complete!"
echo ""
echo "You can now run devproxy using:"
echo "  devproxy start    # Run as background daemon"
echo "  devproxy run      # Run in foreground"
