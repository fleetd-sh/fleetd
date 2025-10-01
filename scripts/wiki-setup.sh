#!/bin/bash
set -e

# fleetd Wiki Setup Script
# This script helps set up and manage the fleetd documentation wiki

WIKI_REPO="git@github.com:fleetd-sh/fleetd.wiki.git"
WIKI_DIR="wiki"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_info() {
    echo -e "${YELLOW}ℹ${NC} $1"
}

# Function to check if wiki repo exists on GitHub
check_wiki_exists() {
    if git ls-remote "$WIKI_REPO" HEAD &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Function to set up wiki as submodule
setup_submodule() {
    print_info "Setting up wiki as git submodule..."

    # Check if submodule already exists
    if git submodule status 2>/dev/null | grep -q "$WIKI_DIR"; then
        print_info "Wiki submodule already exists"
        print_info "Updating submodule..."
        git submodule update --init --recursive
        print_success "Wiki submodule updated"
    else
        # Remove existing wiki directory if it exists
        if [ -d "$WIKI_DIR" ]; then
            print_info "Backing up existing wiki directory to wiki.backup..."
            mv "$WIKI_DIR" wiki.backup
        fi

        # Add submodule
        print_info "Adding wiki as submodule..."
        git submodule add "$WIKI_REPO" "$WIKI_DIR"
        print_success "Wiki added as submodule"

        # Restore backed up content if it exists
        if [ -d "wiki.backup" ]; then
            print_info "Restoring wiki content from backup..."
            cp -r wiki.backup/* "$WIKI_DIR/"
            rm -rf wiki.backup
            print_success "Wiki content restored"
        fi
    fi
}

# Function to push wiki changes
push_wiki() {
    print_info "Pushing wiki changes to GitHub..."

    cd "$WIKI_DIR"

    # Check for changes
    if [ -z "$(git status --porcelain)" ]; then
        print_info "No changes to push"
        return 0
    fi

    # Add and commit changes
    git add .
    git commit -m "Update wiki documentation $(date +%Y-%m-%d)"

    # Push changes
    if git push origin master 2>/dev/null || git push origin main 2>/dev/null; then
        print_success "Wiki changes pushed successfully"
    else
        print_error "Failed to push wiki changes"
        return 1
    fi

    cd ..
}

# Function to create wiki on GitHub
create_wiki_instructions() {
    print_info "The GitHub wiki doesn't exist yet. To create it:"
    echo ""
    echo "  1. Go to https://github.com/fleetd-sh/fleetd"
    echo "  2. Click on the 'Wiki' tab"
    echo "  3. Click 'Create the first page'"
    echo "  4. Add any content and save (you can replace it later)"
    echo "  5. Run this script again"
    echo ""
}

# Main menu
show_menu() {
    echo ""
    echo "fleetd Wiki Management"
    echo "====================="
    echo ""
    echo "1) Check if GitHub wiki exists"
    echo "2) Set up wiki as git submodule"
    echo "3) Push local wiki changes to GitHub"
    echo "4) Pull latest wiki changes from GitHub"
    echo "5) Preview wiki locally (requires gollum)"
    echo "6) Exit"
    echo ""
    read -p "Select an option: " choice
}

# Main script
main() {
    while true; do
        show_menu

        case $choice in
            1)
                print_info "Checking if GitHub wiki exists..."
                if check_wiki_exists; then
                    print_success "GitHub wiki exists and is accessible"
                else
                    print_error "GitHub wiki not found"
                    create_wiki_instructions
                fi
                ;;
            2)
                if check_wiki_exists; then
                    setup_submodule
                else
                    print_error "GitHub wiki doesn't exist yet"
                    create_wiki_instructions
                fi
                ;;
            3)
                if [ -d "$WIKI_DIR/.git" ]; then
                    push_wiki
                else
                    print_error "Wiki is not set up as a git repository"
                    print_info "Run option 2 to set up wiki as submodule first"
                fi
                ;;
            4)
                if [ -d "$WIKI_DIR/.git" ]; then
                    print_info "Pulling latest wiki changes..."
                    cd "$WIKI_DIR"
                    git pull
                    print_success "Wiki updated from GitHub"
                    cd ..
                else
                    print_error "Wiki is not set up as a git repository"
                    print_info "Run option 2 to set up wiki as submodule first"
                fi
                ;;
            5)
                if command -v gollum &> /dev/null; then
                    print_info "Starting Gollum wiki server..."
                    print_info "Open http://localhost:4567 in your browser"
                    print_info "Press Ctrl+C to stop the server"
                    cd "$WIKI_DIR"
                    gollum
                else
                    print_error "Gollum is not installed"
                    print_info "Install it with: gem install gollum"
                fi
                ;;
            6)
                print_info "Goodbye!"
                exit 0
                ;;
            *)
                print_error "Invalid option"
                ;;
        esac

        echo ""
        read -p "Press Enter to continue..."
    done
}

# Run main function
main
