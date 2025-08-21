 Summary

  ✅ Completed Features

  1. Extensible OS Image System
    - ImageProvider interface for plug-and-play OS support
    - ImageManager for downloading and caching images
    - Progress reporting during downloads
  2. DietPi Support
    - Downloads DietPi images from dietpi.com
    - Configures dietpi.txt for automated setup
    - Sets up WiFi, SSH, hostname, and fleetd
    - Optionally installs k3s based on plugin configuration
  3. Raspberry Pi OS Support
    - Downloads official Raspberry Pi OS images
    - Configures wpa_supplicant for WiFi
    - Sets up SSH, hostname, and fleetd
    - First-run script for automatic fleetd installation
  4. SD Card Writer
    - Writes images directly to SD cards
    - Supports multiple compression formats (xz, gz, 7z, zip, zstd)
    - Mounts/unmounts partitions automatically
    - Cross-platform support (macOS/Linux)
  5. Progress Reporting
    - Real-time progress during download
    - Progress during image writing
    - Status updates for each stage
  6. Dry-run Mode
    - Test provisioning without actual writes
    - Useful for validating configuration

  Usage Examples

  # Install p7zip first (for DietPi's 7z format)
  brew install p7zip  # macOS
  # or
  sudo apt-get install p7zip-full  # Linux

  # Provision DietPi with k3s server
  fleetp -device /dev/disk2 -device-type dietpi \
    -wifi-ssid "MyNetwork" -wifi-pass "password" \
    -ssh-key ~/.ssh/id_rsa.pub \
    -plugin k3s -plugin-opt k3s.role=server

  # Provision Raspberry Pi OS
  fleetp -device /dev/disk2 -device-type rpi \
    -wifi-ssid "MyNetwork" -wifi-pass "password" \
    -ssh-key ~/.ssh/id_rsa.pub

  # Dry-run to test without writing
  fleetp -device /dev/disk2 -device-type dietpi \
    -wifi-ssid "MyNetwork" -wifi-pass "password" \
    -dry-run -v

  The system will:
  1. Download the OS image (cached for future use)
  2. Write it to the SD card
  3. Mount partitions
  4. Configure network, SSH, and fleetd
  5. Add k3s installation if requested
  6. Unmount and complete

  After provisioning, insert the SD card into your Raspberry Pi and it will automatically:
  - Connect to WiFi
  - Install fleetd agent
  - Install k3s (if configured)
  - Register with fleet server via mDNS
