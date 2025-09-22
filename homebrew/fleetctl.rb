# FleetD CLI Homebrew Formula
# To install: brew tap fleetd/tap && brew install fleetctl

class Fleetctl < Formula
  desc "Command-line interface for FleetD fleet management platform"
  homepage "https://fleetd.sh"
  version "0.5.2"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/fleetd/fleetd/releases/download/v#{version}/fleetctl_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"
    else
      url "https://github.com/fleetd/fleetd/releases/download/v#{version}/fleetctl_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      if Hardware::CPU.is_64_bit?
        url "https://github.com/fleetd/fleetd/releases/download/v#{version}/fleetctl_#{version}_linux_arm64.tar.gz"
        sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
      else
        url "https://github.com/fleetd/fleetd/releases/download/v#{version}/fleetctl_#{version}_linux_arm.tar.gz"
        sha256 "PLACEHOLDER_SHA256_LINUX_ARM"
      end
    else
      if Hardware::CPU.is_64_bit?
        url "https://github.com/fleetd/fleetd/releases/download/v#{version}/fleetctl_#{version}_linux_amd64.tar.gz"
        sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
      else
        url "https://github.com/fleetd/fleetd/releases/download/v#{version}/fleetctl_#{version}_linux_386.tar.gz"
        sha256 "PLACEHOLDER_SHA256_LINUX_386"
      end
    end
  end

  depends_on "docker" => :recommended

  def install
    bin.install "fleetctl"
    
    # Install shell completions
    generate_completions_from_executable(bin/"fleetctl", "completion")
  end

  def post_install
    # Create config directory
    (var/"fleetd").mkpath
  end

  def caveats
    <<~EOS
      FleetD CLI has been installed successfully!
      
      To get started:
        fleetctl init     # Initialize a new FleetD project
        fleetctl start    # Start local FleetD stack
        
      Docker is recommended for local development:
        brew install docker
        
      Configuration files are stored in:
        #{var}/fleetd
    EOS
  end

  test do
    system "#{bin}/fleetctl", "version"
  end

  service do
    run [opt_bin/"fleetctl", "agent", "--config", etc/"fleetd/agent.yaml"]
    keep_alive true
    log_path var/"log/fleetd.log"
    error_log_path var/"log/fleetd.error.log"
  end
end
