package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Role represents a user role
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleOperator  Role = "operator"
	RoleViewer    Role = "viewer"
	RoleDevice    Role = "device"
	RoleService   Role = "service"
	RoleAnonymous Role = "anonymous"
)

// Permission represents an action permission
type Permission string

const (
	// Device permissions
	PermissionDeviceList      Permission = "device:list"
	PermissionDeviceView      Permission = "device:view"
	PermissionDeviceCreate    Permission = "device:create"
	PermissionDeviceUpdate    Permission = "device:update"
	PermissionDeviceDelete    Permission = "device:delete"
	PermissionDeviceRegister  Permission = "device:register"
	PermissionDeviceHeartbeat Permission = "device:heartbeat"

	// Update permissions
	PermissionUpdateList     Permission = "update:list"
	PermissionUpdateView     Permission = "update:view"
	PermissionUpdateCreate   Permission = "update:create"
	PermissionUpdateApprove  Permission = "update:approve"
	PermissionUpdateDeploy   Permission = "update:deploy"
	PermissionUpdateRollback Permission = "update:rollback"

	// Analytics permissions
	PermissionAnalyticsView   Permission = "analytics:view"
	PermissionAnalyticsExport Permission = "analytics:export"

	// System permissions
	PermissionSystemConfig  Permission = "system:config"
	PermissionSystemBackup  Permission = "system:backup"
	PermissionSystemRestore Permission = "system:restore"

	// User permissions
	PermissionUserList   Permission = "user:list"
	PermissionUserView   Permission = "user:view"
	PermissionUserCreate Permission = "user:create"
	PermissionUserUpdate Permission = "user:update"
	PermissionUserDelete Permission = "user:delete"
)

// RolePermissions defines permissions for each role
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		// All permissions
		PermissionDeviceList, PermissionDeviceView, PermissionDeviceCreate,
		PermissionDeviceUpdate, PermissionDeviceDelete, PermissionDeviceRegister,
		PermissionDeviceHeartbeat,
		PermissionUpdateList, PermissionUpdateView, PermissionUpdateCreate,
		PermissionUpdateApprove, PermissionUpdateDeploy, PermissionUpdateRollback,
		PermissionAnalyticsView, PermissionAnalyticsExport,
		PermissionSystemConfig, PermissionSystemBackup, PermissionSystemRestore,
		PermissionUserList, PermissionUserView, PermissionUserCreate,
		PermissionUserUpdate, PermissionUserDelete,
	},
	RoleOperator: {
		// Device and update management
		PermissionDeviceList, PermissionDeviceView, PermissionDeviceUpdate,
		PermissionUpdateList, PermissionUpdateView, PermissionUpdateCreate,
		PermissionUpdateDeploy, PermissionUpdateRollback,
		PermissionAnalyticsView,
		PermissionUserList, PermissionUserView,
	},
	RoleViewer: {
		// Read-only access
		PermissionDeviceList, PermissionDeviceView,
		PermissionUpdateList, PermissionUpdateView,
		PermissionAnalyticsView,
		PermissionUserList, PermissionUserView,
	},
	RoleDevice: {
		// Device self-management
		PermissionDeviceRegister, PermissionDeviceHeartbeat,
		PermissionDeviceView, // Can view self
		PermissionUpdateView, // Can view available updates
	},
	RoleService: {
		// Service-to-service communication
		PermissionDeviceList, PermissionDeviceView,
		PermissionUpdateList, PermissionUpdateView,
		PermissionAnalyticsView,
	},
	RoleAnonymous: {
		// No permissions by default
	},
}

// User represents an authenticated user
type User struct {
	ID          string         `json:"id"`
	Username    string         `json:"username"`
	Email       string         `json:"email"`
	Roles       []Role         `json:"roles"`
	Permissions []Permission   `json:"permissions"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	LastLogin   time.Time      `json:"last_login"`
}

// HasPermission checks if user has a specific permission
func (u *User) HasPermission(permission Permission) bool {
	// Check direct permissions
	for _, p := range u.Permissions {
		if p == permission {
			return true
		}
	}

	// Check role-based permissions
	for _, role := range u.Roles {
		if perms, exists := RolePermissions[role]; exists {
			for _, p := range perms {
				if p == permission {
					return true
				}
			}
		}
	}

	return false
}

// HasRole checks if user has a specific role
func (u *User) HasRole(role Role) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsAdmin checks if user is an admin
func (u *User) IsAdmin() bool {
	return u.HasRole(RoleAdmin)
}

// RBACManager manages role-based access control
type RBACManager struct {
	users    map[string]*User
	policies map[string]*Policy
	mu       sync.RWMutex
	logger   *slog.Logger
	cache    *PermissionCache
}

// Policy represents an access control policy
type Policy struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Resource    string       `json:"resource"`
	Actions     []string     `json:"actions"`
	Effect      PolicyEffect `json:"effect"`
	Conditions  []Condition  `json:"conditions"`
	Priority    int          `json:"priority"`
}

// PolicyEffect represents the effect of a policy
type PolicyEffect string

const (
	PolicyEffectAllow PolicyEffect = "allow"
	PolicyEffectDeny  PolicyEffect = "deny"
)

// Condition represents a policy condition
type Condition struct {
	Type     string `json:"type"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

// PermissionCache caches permission checks
type PermissionCache struct {
	cache map[string]bool
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager() *RBACManager {
	return &RBACManager{
		users:    make(map[string]*User),
		policies: make(map[string]*Policy),
		logger:   slog.Default().With("component", "rbac"),
		cache: &PermissionCache{
			cache: make(map[string]bool),
			ttl:   5 * time.Minute,
		},
	}
}

// CreateUser creates a new user
func (m *RBACManager) CreateUser(user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if user.ID == "" {
		return errors.New("user ID is required")
	}

	if _, exists := m.users[user.ID]; exists {
		return fmt.Errorf("user already exists: %s", user.ID)
	}

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	m.users[user.ID] = user
	m.logger.Info("User created",
		"user_id", user.ID,
		"username", user.Username,
		"roles", user.Roles,
	)

	return nil
}

// GetUser retrieves a user by ID
func (m *RBACManager) GetUser(userID string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, exists := m.users[userID]
	if !exists {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	return user, nil
}

// UpdateUser updates a user
func (m *RBACManager) UpdateUser(user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[user.ID]; !exists {
		return fmt.Errorf("user not found: %s", user.ID)
	}

	user.UpdatedAt = time.Now()
	m.users[user.ID] = user

	// Clear cache for this user
	m.cache.Clear(user.ID)

	m.logger.Info("User updated",
		"user_id", user.ID,
		"username", user.Username,
		"roles", user.Roles,
	)

	return nil
}

// DeleteUser deletes a user
func (m *RBACManager) DeleteUser(userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[userID]; !exists {
		return fmt.Errorf("user not found: %s", userID)
	}

	delete(m.users, userID)
	m.cache.Clear(userID)

	m.logger.Info("User deleted", "user_id", userID)
	return nil
}

// CheckPermission checks if a user has permission for an action
func (m *RBACManager) CheckPermission(ctx context.Context, userID string, permission Permission) error {
	// Check cache first
	if allowed := m.cache.Get(userID, string(permission)); allowed != nil {
		if *allowed {
			return nil
		}
		return errors.New("permission denied")
	}

	// Get user
	user, err := m.GetUser(userID)
	if err != nil {
		return err
	}

	// Check permission
	allowed := user.HasPermission(permission)

	// Cache result
	m.cache.Set(userID, string(permission), allowed)

	if !allowed {
		m.logger.Warn("Permission denied",
			"user_id", userID,
			"permission", permission,
			"user_roles", user.Roles,
		)
		return fmt.Errorf("user %s does not have permission %s", userID, permission)
	}

	return nil
}

// CheckPolicy evaluates a policy for a user
func (m *RBACManager) CheckPolicy(ctx context.Context, userID string, resource string, action string) error {
	user, err := m.GetUser(userID)
	if err != nil {
		return err
	}

	m.mu.RLock()
	policies := make([]*Policy, 0, len(m.policies))
	for _, policy := range m.policies {
		if policy.Resource == resource {
			policies = append(policies, policy)
		}
	}
	m.mu.RUnlock()

	// Sort policies by priority
	// Higher priority policies are evaluated first
	// Explicit deny always takes precedence

	var explicitDeny bool
	var explicitAllow bool

	for _, policy := range policies {
		// Check if action matches
		actionMatches := false
		for _, a := range policy.Actions {
			if a == action || a == "*" {
				actionMatches = true
				break
			}
		}

		if !actionMatches {
			continue
		}

		// Evaluate conditions
		conditionsMet := true
		for _, condition := range policy.Conditions {
			if !m.evaluateCondition(ctx, user, condition) {
				conditionsMet = false
				break
			}
		}

		if !conditionsMet {
			continue
		}

		// Apply policy effect
		if policy.Effect == PolicyEffectDeny {
			explicitDeny = true
			break // Deny takes precedence
		} else if policy.Effect == PolicyEffectAllow {
			explicitAllow = true
		}
	}

	if explicitDeny {
		return fmt.Errorf("access denied by policy for resource %s action %s", resource, action)
	}

	if !explicitAllow {
		return fmt.Errorf("no policy allows access to resource %s action %s", resource, action)
	}

	return nil
}

// evaluateCondition evaluates a policy condition
func (m *RBACManager) evaluateCondition(ctx context.Context, user *User, condition Condition) bool {
	switch condition.Type {
	case "role":
		return m.evaluateRoleCondition(user, condition)
	case "time":
		return m.evaluateTimeCondition(condition)
	case "ip":
		return m.evaluateIPCondition(ctx, condition)
	default:
		return false
	}
}

func (m *RBACManager) evaluateRoleCondition(user *User, condition Condition) bool {
	requiredRole, ok := condition.Value.(string)
	if !ok {
		return false
	}

	switch condition.Operator {
	case "equals":
		return user.HasRole(Role(requiredRole))
	case "not_equals":
		return !user.HasRole(Role(requiredRole))
	default:
		return false
	}
}

func (m *RBACManager) evaluateTimeCondition(condition Condition) bool {
	// Implement time-based conditions
	// For example, allow access only during business hours
	return true
}

func (m *RBACManager) evaluateIPCondition(ctx context.Context, condition Condition) bool {
	// Implement IP-based conditions
	// For example, allow access only from specific IP ranges
	return true
}

// CreatePolicy creates a new policy
func (m *RBACManager) CreatePolicy(policy *Policy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if policy.ID == "" {
		return errors.New("policy ID is required")
	}

	if _, exists := m.policies[policy.ID]; exists {
		return fmt.Errorf("policy already exists: %s", policy.ID)
	}

	m.policies[policy.ID] = policy
	m.logger.Info("Policy created",
		"policy_id", policy.ID,
		"resource", policy.Resource,
		"effect", policy.Effect,
	)

	return nil
}

// PermissionCache methods

func (c *PermissionCache) Get(userID, permission string) *bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", userID, permission)
	if val, exists := c.cache[key]; exists {
		return &val
	}
	return nil
}

func (c *PermissionCache) Set(userID, permission string, allowed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s:%s", userID, permission)
	c.cache[key] = allowed
}

func (c *PermissionCache) Clear(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear all cache entries for this user
	for key := range c.cache {
		if len(key) > len(userID) && key[:len(userID)] == userID {
			delete(c.cache, key)
		}
	}
}

// GetPermissionsForRole returns all permissions for a role
func GetPermissionsForRole(role Role) []Permission {
	return RolePermissions[role]
}

// ValidateRole checks if a role is valid
func ValidateRole(role Role) bool {
	_, exists := RolePermissions[role]
	return exists
}
