package container

import (
	"context"
	"io"
	"time"
)

// ContainerState represents the state of a container
type ContainerState string

const (
	ContainerStateCreated    ContainerState = "created"
	ContainerStateRunning    ContainerState = "running"
	ContainerStatePaused     ContainerState = "paused"
	ContainerStateStopped    ContainerState = "stopped"
	ContainerStateRestarting ContainerState = "restarting"
	ContainerStateRemoving   ContainerState = "removing"
	ContainerStateExited     ContainerState = "exited"
	ContainerStateDead       ContainerState = "dead"
	ContainerStateUnknown    ContainerState = "unknown"
)

// HealthState represents the health state of a container
type HealthState string

const (
	HealthStateHealthy   HealthState = "healthy"
	HealthStateUnhealthy HealthState = "unhealthy"
	HealthStateStarting  HealthState = "starting"
	HealthStateNone      HealthState = "none"
)

// Mount represents a container mount point
type Mount struct {
	Source      string            // Source path on host
	Target      string            // Target path in container
	Type        string            // Mount type (bind, volume, tmpfs)
	ReadOnly    bool              // Read-only mount
	Propagation string            // Mount propagation
	Options     map[string]string // Mount options
}

// NetworkConfig represents container network configuration
type NetworkConfig struct {
	NetworkMode string            // Network mode (bridge, host, none)
	Networks    []string          // Networks to connect to
	Ports       map[string]string // Port mappings (host:container)
	DNS         []string          // DNS servers
	DNSSearch   []string          // DNS search domains
	ExtraHosts  []string          // Extra /etc/hosts entries
}

// Resources represents container resource limits
type Resources struct {
	CPUShares         int64
	CPUQuota          int64
	CPUPeriod         int64
	Memory            int64
	MemorySwap        int64
	MemoryReservation int64
	OOMKillDisable    bool
	PidsLimit         int64
}

// LogConfig represents container logging configuration
type LogConfig struct {
	Driver  string            // Log driver (json-file, syslog, etc.)
	Options map[string]string // Log driver options
}

// ContainerConfig represents container configuration
type ContainerConfig struct {
	Name          string
	Image         string
	Command       []string
	Entrypoint    []string
	Env           []string
	Labels        map[string]string
	Mounts        []Mount
	Network       NetworkConfig
	Resources     Resources
	LogConfig     LogConfig
	RestartPolicy string // no, always, unless-stopped, on-failure[:max-retries]
	StopTimeout   time.Duration
}

// ContainerInfo represents container information
type ContainerInfo struct {
	ID            string
	Name          string
	Image         string
	State         ContainerState
	Health        HealthState
	CreatedAt     time.Time
	StartedAt     time.Time
	FinishedAt    time.Time
	ExitCode      int
	RestartCount  int
	Platform      string
	Driver        string
	NetworkMode   string
	IPAddress     string
	Ports         map[string]string
	Mounts        []Mount
	Labels        map[string]string
	RestartPolicy string
}

// ContainerStats represents container statistics
type ContainerStats struct {
	CPUPercentage    float64
	MemoryUsage      uint64
	MemoryLimit      uint64
	MemoryPercentage float64
	NetworkRx        uint64
	NetworkTx        uint64
	BlockRead        uint64
	BlockWrite       uint64
	PIDs             uint64
	Timestamp        time.Time
}

// ContainerManager defines the interface for container operations
type ContainerManager interface {
	// Initialize initializes the container manager
	Initialize(ctx context.Context) error

	// ListContainers lists containers matching the filter
	ListContainers(ctx context.Context, filters map[string]string) ([]ContainerInfo, error)

	// InspectContainer gets detailed container information
	InspectContainer(ctx context.Context, nameOrID string) (*ContainerInfo, error)

	// CreateContainer creates a new container
	CreateContainer(ctx context.Context, config ContainerConfig) (string, error)

	// StartContainer starts a container
	StartContainer(ctx context.Context, nameOrID string) error

	// StopContainer stops a container
	StopContainer(ctx context.Context, nameOrID string, timeout time.Duration) error

	// RestartContainer restarts a container
	RestartContainer(ctx context.Context, nameOrID string, timeout time.Duration) error

	// PauseContainer pauses a container
	PauseContainer(ctx context.Context, nameOrID string) error

	// UnpauseContainer unpauses a container
	UnpauseContainer(ctx context.Context, nameOrID string) error

	// RemoveContainer removes a container
	RemoveContainer(ctx context.Context, nameOrID string, force bool) error

	// GetContainerLogs gets container logs
	GetContainerLogs(ctx context.Context, nameOrID string, since time.Time) (io.ReadCloser, error)

	// GetContainerStats gets container statistics
	GetContainerStats(ctx context.Context, nameOrID string) (*ContainerStats, error)

	// ExecInContainer executes a command in a container
	ExecInContainer(ctx context.Context, nameOrID string, cmd []string, attachStdio bool) (int, error)

	// CopyToContainer copies files/folders to a container
	CopyToContainer(ctx context.Context, nameOrID, path string, content io.Reader) error

	// CopyFromContainer copies files/folders from a container
	CopyFromContainer(ctx context.Context, nameOrID, path string) (io.ReadCloser, error)

	// PruneContainers removes stopped containers
	PruneContainers(ctx context.Context) error

	// Events returns a channel for container events
	Events(ctx context.Context) (<-chan ContainerEvent, error)

	// Close closes the container manager
	Close() error
}

type ContainerEventType string

const (
	ContainerEventCreate  ContainerEventType = "create"
	ContainerEventStart   ContainerEventType = "start"
	ContainerEventStop    ContainerEventType = "stop"
	ContainerEventDie     ContainerEventType = "die"
	ContainerEventDestroy ContainerEventType = "destroy"
	// Add other event types as needed
)

// ContainerEvent represents a container event
type ContainerEvent struct {
	Type       string // create, start, stop, die, destroy, etc.
	ID         string
	Name       string
	Image      string
	Time       time.Time
	Status     ContainerState
	ExitCode   int
	Error      string
	Attributes map[string]string
}

// ContainerFactory creates container managers
type ContainerFactory interface {
	// Create creates a container manager for the given runtime
	Create(runtime string, options map[string]interface{}) (ContainerManager, error)
}

// RegisterContainerFactory registers a container factory
func RegisterContainerFactory(name string, factory ContainerFactory) {
	containerFactories[name] = factory
}

// GetContainerFactory gets a registered container factory
func GetContainerFactory(name string) (ContainerFactory, bool) {
	factory, ok := containerFactories[name]
	return factory, ok
}

var containerFactories = make(map[string]ContainerFactory)
