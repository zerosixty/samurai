#!/bin/zsh
# install-plugin.sh - Build and install Samurai Test Runner plugin to GoLand
#
# Usage: ./install-plugin.sh
#
# This script builds the plugin and installs it to your local GoLand installation.
# After running, restart GoLand to activate the plugin.

set -e

PLUGIN_NAME="samurai-goland"
GOLAND_PLUGINS_DIR=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo_info() {
    echo "${GREEN}[INFO]${NC} $1"
}

echo_warn() {
    echo "${YELLOW}[WARN]${NC} $1"
}

echo_error() {
    echo "${RED}[ERROR]${NC} $1"
}

echo_step() {
    echo "${BLUE}==>${NC} $1"
}

# Detect GoLand installation
detect_goland() {
    local candidates=()

    # macOS locations
    if [[ "$OSTYPE" == "darwin"* ]]; then
        for version in 2025.3; do
            candidates+=("$HOME/Library/Application Support/JetBrains/GoLand${version}/plugins")
        done
    # Linux locations
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        for version in 2025.3; do
            candidates+=("$HOME/.local/share/JetBrains/GoLand${version}/plugins")
            candidates+=("$HOME/.config/JetBrains/GoLand${version}/plugins")
        done
    # Windows (Git Bash / WSL)
    else
        for version in 2025.3; do
            candidates+=("$APPDATA/JetBrains/GoLand${version}/plugins")
        done
    fi

    for dir in "${candidates[@]}"; do
        parent_dir="$(dirname "$dir")"
        if [[ -d "$parent_dir" ]]; then
            GOLAND_PLUGINS_DIR="$dir"
            return 0
        fi
    done

    return 1
}

# Check Java (Gradle toolchain will download the right version if needed)
check_java() {
    echo_step "Checking Java..."

    if ! command -v java &> /dev/null; then
        echo_warn "Java not found in PATH. Gradle will use its toolchain to download Java 21."
    else
        local java_version=$(java -version 2>&1 | head -1 | cut -d'"' -f2 | cut -d'.' -f1)
        echo_info "System Java: $java_version (Gradle toolchain will use Java 21)"
    fi
}

# Build the plugin
build_plugin() {
    echo_step "Building plugin..."

    if [[ ! -f "./gradlew" ]]; then
        echo_error "gradlew not found. Are you in the plugin-goland directory?"
        exit 1
    fi

    # Make gradlew executable
    chmod +x ./gradlew

    ./gradlew clean buildPlugin --no-daemon

    # Find the built distribution
    local dist_dir="build/distributions"
    BUILT_ZIP_FILE=$(ls -1 "$dist_dir"/*.zip 2>/dev/null | head -1)

    if [[ -z "$BUILT_ZIP_FILE" ]]; then
        echo_error "Plugin ZIP not found in $dist_dir"
        exit 1
    fi

    echo_info "Built: $BUILT_ZIP_FILE"
}

# Install to GoLand
install_plugin() {
    local zip_file="$1"

    echo_step "Installing plugin..."

    # Create plugins directory if needed
    mkdir -p "$GOLAND_PLUGINS_DIR"

    # Remove old version
    local plugin_dir="$GOLAND_PLUGINS_DIR/$PLUGIN_NAME"
    if [[ -d "$plugin_dir" ]]; then
        echo_info "Removing old version..."
        rm -rf "$plugin_dir"
    fi

    # Extract new version
    echo_info "Installing to $GOLAND_PLUGINS_DIR..."
    unzip -q "$zip_file" -d "$GOLAND_PLUGINS_DIR"

    echo ""
    echo "${GREEN}============================================${NC}"
    echo "${GREEN}  SUCCESS! Plugin installed.${NC}"
    echo "${GREEN}============================================${NC}"
    echo ""
    echo "  Plugin location: $plugin_dir"
    echo ""
    echo "  ${YELLOW}Restart GoLand to activate the plugin.${NC}"
    echo ""
}

# Uninstall plugin
uninstall_plugin() {
    echo_step "Uninstalling plugin..."

    if [[ -z "$GOLAND_PLUGINS_DIR" ]]; then
        if ! detect_goland; then
            echo_error "Could not detect GoLand installation."
            exit 1
        fi
    fi

    local plugin_dir="$GOLAND_PLUGINS_DIR/$PLUGIN_NAME"
    if [[ -d "$plugin_dir" ]]; then
        rm -rf "$plugin_dir"
        echo_info "Plugin uninstalled from $plugin_dir"
        echo_warn "Restart GoLand to complete uninstallation."
    else
        echo_warn "Plugin not found at $plugin_dir"
    fi
}

# Show help
show_help() {
    echo "Samurai GoLand Plugin Installer"
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  install     Build and install the plugin (default)"
    echo "  uninstall   Remove the plugin from GoLand"
    echo "  build       Build the plugin only (no install)"
    echo "  help        Show this help message"
    echo ""
    echo "Environment Variables:"
    echo "  GOLAND_PLUGINS_DIR   Override auto-detected plugins directory"
    echo ""
    echo "Examples:"
    echo "  $0                    # Build and install"
    echo "  $0 build              # Build only"
    echo "  $0 uninstall          # Remove plugin"
    echo ""
}

# Main
main() {
    local command="${1:-install}"

    echo ""
    echo "${BLUE}=== Samurai GoLand Plugin ===${NC}"
    echo ""

    case "$command" in
        help|-h|--help)
            show_help
            exit 0
            ;;
        uninstall)
            uninstall_plugin
            exit 0
            ;;
        build)
            check_java
            build_plugin
            exit 0
            ;;
        install)
            # Check for manual override
            if [[ -n "$GOLAND_PLUGINS_DIR" && "$GOLAND_PLUGINS_DIR" != "" ]]; then
                echo_info "Using GOLAND_PLUGINS_DIR from environment"
            else
                echo_step "Detecting GoLand installation..."
                if detect_goland; then
                    echo_info "Found: $GOLAND_PLUGINS_DIR"
                else
                    echo_error "Could not detect GoLand installation."
                    echo_warn "Please set GOLAND_PLUGINS_DIR environment variable manually."
                    echo ""
                    echo "Example:"
                    echo "  export GOLAND_PLUGINS_DIR=\"\$HOME/Library/Application Support/JetBrains/GoLand2025.3/plugins\""
                    echo "  ./install-plugin.sh"
                    exit 1
                fi
            fi

            echo ""
            check_java
            build_plugin
            install_plugin "$BUILT_ZIP_FILE"
            ;;
        *)
            echo_error "Unknown command: $command"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
