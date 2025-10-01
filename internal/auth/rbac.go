package auth

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// RBAC implements Role-Based Access Control
type RBAC struct {
	db    *sql.DB
	cache PermissionCache // Optional cache for performance
}

// PermissionCache interface for caching permissions
type PermissionCache interface {
	Get(key string) ([]string, bool)
	Set(key string, permissions []string)
	Delete(key string)
}

// Role represents a user role
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	CreatedAt   string   `json:"created_at"`
}

// Permission represents a system permission
type Permission struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

// Predefined roles
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
	RoleDevice   = "device"
)

// Predefined permissions
const (
	// Deployment permissions
	PermDeploymentCreate = "deployment:create"
	PermDeploymentRead   = "deployment:read"
	PermDeploymentUpdate = "deployment:update"
	PermDeploymentDelete = "deployment:delete"
	PermDeploymentApprove = "deployment:approve"

	// Device permissions
	PermDeviceCreate = "device:create"
	PermDeviceRead   = "device:read"
	PermDeviceUpdate = "device:update"
	PermDeviceDelete = "device:delete"
	PermDeviceCommand = "device:command"

	// Fleet permissions
	PermFleetManage = "fleet:manage"
	PermFleetView   = "fleet:view"

	// System permissions
	PermSystemAdmin = "system:admin"
	PermSystemConfig = "system:config"
	PermSystemMetrics = "system:metrics"

	// API permissions
	PermAPIAccess = "api:access"
	PermAPIAdmin  = "api:admin"
)

// NewRBAC creates a new RBAC manager
func NewRBAC(db *sql.DB, cache PermissionCache) *RBAC {
	return &RBAC{
		db:    db,
		cache: cache,
	}
}

// InitializeRoles creates default roles and permissions
func (r *RBAC) InitializeRoles(ctx context.Context) error {
	// Define default roles with permissions
	defaultRoles := []struct {
		role        string
		description string
		permissions []string
	}{
		{
			role:        RoleAdmin,
			description: "Full system administrator",
			permissions: []string{
				PermDeploymentCreate, PermDeploymentRead, PermDeploymentUpdate, PermDeploymentDelete, PermDeploymentApprove,
				PermDeviceCreate, PermDeviceRead, PermDeviceUpdate, PermDeviceDelete, PermDeviceCommand,
				PermFleetManage, PermFleetView,
				PermSystemAdmin, PermSystemConfig, PermSystemMetrics,
				PermAPIAccess, PermAPIAdmin,
			},
		},
		{
			role:        RoleOperator,
			description: "Deployment operator",
			permissions: []string{
				PermDeploymentCreate, PermDeploymentRead, PermDeploymentUpdate,
				PermDeviceRead, PermDeviceUpdate, PermDeviceCommand,
				PermFleetManage, PermFleetView,
				PermSystemMetrics,
				PermAPIAccess,
			},
		},
		{
			role:        RoleViewer,
			description: "Read-only access",
			permissions: []string{
				PermDeploymentRead,
				PermDeviceRead,
				PermFleetView,
				PermSystemMetrics,
				PermAPIAccess,
			},
		},
		{
			role:        RoleDevice,
			description: "Device authentication role",
			permissions: []string{
				PermDeviceRead,
				PermDeviceUpdate,
				PermAPIAccess,
			},
		},
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Create roles
	for _, dr := range defaultRoles {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO roles (id, name, description)
			VALUES (?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET description = excluded.description`,
			dr.role, dr.role, dr.description)
		if err != nil {
			return fmt.Errorf("failed to create role %s: %w", dr.role, err)
		}

		// Assign permissions to role
		for _, perm := range dr.permissions {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO role_permissions (role_id, permission)
				VALUES (?, ?)
				ON CONFLICT DO NOTHING`,
				dr.role, perm)
			if err != nil {
				return fmt.Errorf("failed to assign permission %s to role %s: %w", perm, dr.role, err)
			}
		}
	}

	return tx.Commit()
}

// CheckPermission checks if a user has a specific permission
func (r *RBAC) CheckPermission(ctx context.Context, userID, permission string) (bool, error) {
	// Check cache first
	if r.cache != nil {
		cacheKey := fmt.Sprintf("user:%s:perms", userID)
		if perms, found := r.cache.Get(cacheKey); found {
			return r.hasPermission(perms, permission), nil
		}
	}

	// Get user permissions from database
	perms, err := r.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}

	// Cache the permissions
	if r.cache != nil {
		cacheKey := fmt.Sprintf("user:%s:perms", userID)
		r.cache.Set(cacheKey, perms)
	}

	return r.hasPermission(perms, permission), nil
}

// GetUserPermissions gets all permissions for a user
func (r *RBAC) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT DISTINCT rp.permission
		FROM user_roles ur
		JOIN role_permissions rp ON ur.role_id = rp.role_id
		WHERE ur.user_id = ?`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}

	return permissions, rows.Err()
}

// GetUserRoles gets all roles for a user
func (r *RBAC) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT role_id
		FROM user_roles
		WHERE user_id = ?`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, rows.Err()
}

// AssignRole assigns a role to a user
func (r *RBAC) AssignRole(ctx context.Context, userID, roleID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING`,
		userID, roleID)

	// Invalidate cache
	if r.cache != nil {
		cacheKey := fmt.Sprintf("user:%s:perms", userID)
		r.cache.Delete(cacheKey)
	}

	return err
}

// RemoveRole removes a role from a user
func (r *RBAC) RemoveRole(ctx context.Context, userID, roleID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM user_roles
		WHERE user_id = ? AND role_id = ?`,
		userID, roleID)

	// Invalidate cache
	if r.cache != nil {
		cacheKey := fmt.Sprintf("user:%s:perms", userID)
		r.cache.Delete(cacheKey)
	}

	return err
}

// CreateCustomRole creates a new custom role
func (r *RBAC) CreateCustomRole(ctx context.Context, role *Role) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Create role
	_, err = tx.ExecContext(ctx, `
		INSERT INTO roles (id, name, description)
		VALUES (?, ?, ?)`,
		role.ID, role.Name, role.Description)
	if err != nil {
		return err
	}

	// Assign permissions
	for _, perm := range role.Permissions {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO role_permissions (role_id, permission)
			VALUES (?, ?)`,
			role.ID, perm)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// hasPermission checks if a permission list contains a specific permission
func (r *RBAC) hasPermission(permissions []string, required string) bool {
	// Check for exact match
	for _, p := range permissions {
		if p == required {
			return true
		}

		// Check for wildcard permissions (e.g., "deployment:*" matches "deployment:create")
		if strings.HasSuffix(p, ":*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(required, prefix) {
				return true
			}
		}

		// Check for admin override
		if p == PermSystemAdmin {
			return true
		}
	}

	return false
}

// CheckMultiplePermissions checks if a user has all specified permissions
func (r *RBAC) CheckMultiplePermissions(ctx context.Context, userID string, permissions []string) (bool, error) {
	userPerms, err := r.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}

	for _, required := range permissions {
		if !r.hasPermission(userPerms, required) {
			return false, nil
		}
	}

	return true, nil
}

// CheckAnyPermission checks if a user has at least one of the specified permissions
func (r *RBAC) CheckAnyPermission(ctx context.Context, userID string, permissions []string) (bool, error) {
	userPerms, err := r.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}

	for _, required := range permissions {
		if r.hasPermission(userPerms, required) {
			return true, nil
		}
	}

	return false, nil
}