package security

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUser_HasPermission(t *testing.T) {
	tests := []struct {
		name       string
		user       *User
		permission Permission
		expected   bool
	}{
		{
			name: "direct permission",
			user: &User{
				ID:          "user1",
				Permissions: []Permission{PermissionDeviceView, PermissionDeviceList},
			},
			permission: PermissionDeviceView,
			expected:   true,
		},
		{
			name: "role-based permission",
			user: &User{
				ID:    "user2",
				Roles: []Role{RoleViewer},
			},
			permission: PermissionDeviceList,
			expected:   true,
		},
		{
			name: "admin has all permissions",
			user: &User{
				ID:    "admin",
				Roles: []Role{RoleAdmin},
			},
			permission: PermissionSystemConfig,
			expected:   true,
		},
		{
			name: "no permission",
			user: &User{
				ID:    "user3",
				Roles: []Role{RoleViewer},
			},
			permission: PermissionDeviceDelete,
			expected:   false,
		},
		{
			name: "combined role and direct permissions",
			user: &User{
				ID:          "user4",
				Roles:       []Role{RoleViewer},
				Permissions: []Permission{PermissionUpdateDeploy},
			},
			permission: PermissionUpdateDeploy,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.user.HasPermission(tt.permission)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUser_HasRole(t *testing.T) {
	user := &User{
		ID:    "user1",
		Roles: []Role{RoleOperator, RoleViewer},
	}

	assert.True(t, user.HasRole(RoleOperator))
	assert.True(t, user.HasRole(RoleViewer))
	assert.False(t, user.HasRole(RoleAdmin))
	assert.False(t, user.HasRole(RoleDevice))
}

func TestUser_IsAdmin(t *testing.T) {
	adminUser := &User{
		ID:    "admin",
		Roles: []Role{RoleAdmin},
	}
	assert.True(t, adminUser.IsAdmin())

	operatorUser := &User{
		ID:    "operator",
		Roles: []Role{RoleOperator},
	}
	assert.False(t, operatorUser.IsAdmin())
}

func TestRBACManager_UserManagement(t *testing.T) {
	manager := NewRBACManager()

	// Create user
	user := &User{
		ID:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
		Roles:    []Role{RoleViewer},
	}

	err := manager.CreateUser(user)
	require.NoError(t, err)

	// Get user
	retrievedUser, err := manager.GetUser("test-user")
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Username, retrievedUser.Username)

	// Update user
	user.Email = "newemail@example.com"
	err = manager.UpdateUser(user)
	require.NoError(t, err)

	// Delete user
	err = manager.DeleteUser("test-user")
	require.NoError(t, err)

	// Verify deletion
	_, err = manager.GetUser("test-user")
	assert.Error(t, err)
}

func TestRBACManager_CheckPermission(t *testing.T) {
	manager := NewRBACManager()
	ctx := context.Background()

	// Create users with different roles
	adminUser := &User{
		ID:    "admin",
		Roles: []Role{RoleAdmin},
	}
	manager.CreateUser(adminUser)

	viewerUser := &User{
		ID:    "viewer",
		Roles: []Role{RoleViewer},
	}
	manager.CreateUser(viewerUser)

	// Test admin permissions
	err := manager.CheckPermission(ctx, "admin", PermissionSystemConfig)
	assert.NoError(t, err)

	// Test viewer permissions
	err = manager.CheckPermission(ctx, "viewer", PermissionDeviceView)
	assert.NoError(t, err)

	// Test denied permission
	err = manager.CheckPermission(ctx, "viewer", PermissionDeviceDelete)
	assert.Error(t, err)
}

func TestRBACManager_PolicyEvaluation(t *testing.T) {
	manager := NewRBACManager()
	ctx := context.Background()

	// Create user
	user := &User{
		ID:    "user1",
		Roles: []Role{RoleOperator},
	}
	manager.CreateUser(user)

	// Create allow policy
	allowPolicy := &Policy{
		ID:       "allow-read",
		Name:     "Allow Read Operations",
		Resource: "/api/devices",
		Actions:  []string{"read", "list"},
		Effect:   PolicyEffectAllow,
		Priority: 10,
	}
	err := manager.CreatePolicy(allowPolicy)
	require.NoError(t, err)

	// Create deny policy with higher priority
	denyPolicy := &Policy{
		ID:       "deny-sensitive",
		Name:     "Deny Sensitive Operations",
		Resource: "/api/devices",
		Actions:  []string{"delete"},
		Effect:   PolicyEffectDeny,
		Priority: 20,
	}
	err = manager.CreatePolicy(denyPolicy)
	require.NoError(t, err)

	// Test allowed action
	err = manager.CheckPolicy(ctx, "user1", "/api/devices", "read")
	assert.NoError(t, err)

	// Test denied action
	err = manager.CheckPolicy(ctx, "user1", "/api/devices", "delete")
	assert.Error(t, err)

	// Test action with no policy
	err = manager.CheckPolicy(ctx, "user1", "/api/unknown", "read")
	assert.Error(t, err)
}

func TestPermissionCache(t *testing.T) {
	cache := &PermissionCache{
		cache: make(map[string]bool),
		ttl:   5 * time.Minute,
	}

	// Set cache value
	cache.Set("user1", "device:view", true)
	cache.Set("user1", "device:delete", false)

	// Get cache value
	val := cache.Get("user1", "device:view")
	require.NotNil(t, val)
	assert.True(t, *val)

	val = cache.Get("user1", "device:delete")
	require.NotNil(t, val)
	assert.False(t, *val)

	// Get non-existent cache value
	val = cache.Get("user1", "device:create")
	assert.Nil(t, val)

	// Clear cache for user
	cache.Clear("user1")
	val = cache.Get("user1", "device:view")
	assert.Nil(t, val)
}

func TestGetPermissionsForRole(t *testing.T) {
	adminPerms := GetPermissionsForRole(RoleAdmin)
	assert.Contains(t, adminPerms, PermissionSystemConfig)
	assert.Contains(t, adminPerms, PermissionDeviceDelete)

	viewerPerms := GetPermissionsForRole(RoleViewer)
	assert.Contains(t, viewerPerms, PermissionDeviceView)
	assert.NotContains(t, viewerPerms, PermissionDeviceDelete)

	devicePerms := GetPermissionsForRole(RoleDevice)
	assert.Contains(t, devicePerms, PermissionDeviceRegister)
	assert.Contains(t, devicePerms, PermissionDeviceHeartbeat)
}

func TestValidateRole(t *testing.T) {
	assert.True(t, ValidateRole(RoleAdmin))
	assert.True(t, ValidateRole(RoleOperator))
	assert.True(t, ValidateRole(RoleViewer))
	assert.False(t, ValidateRole("invalid_role"))
}

// Database store tests

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	return db
}

func TestSQLRBACStore_UserOperations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()

	// Create user
	user := &User{
		ID:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
		Roles:    []Role{RoleViewer, RoleOperator},
		Permissions: []Permission{
			PermissionDeviceView,
			PermissionUpdateDeploy,
		},
		Metadata: map[string]any{
			"department": "engineering",
		},
	}

	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Get user
	retrievedUser, err := store.GetUser(ctx, "test-user")
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Username, retrievedUser.Username)
	assert.Equal(t, user.Email, retrievedUser.Email)
	assert.Contains(t, retrievedUser.Roles, RoleViewer)
	assert.Contains(t, retrievedUser.Roles, RoleOperator)
	assert.Contains(t, retrievedUser.Permissions, PermissionDeviceView)

	// Get user by username
	userByUsername, err := store.GetUserByUsername(ctx, "testuser")
	require.NoError(t, err)
	assert.Equal(t, user.ID, userByUsername.ID)

	// Update user
	user.Email = "newemail@example.com"
	err = store.UpdateUser(ctx, user)
	require.NoError(t, err)

	retrievedUser, err = store.GetUser(ctx, "test-user")
	require.NoError(t, err)
	assert.Equal(t, "newemail@example.com", retrievedUser.Email)

	// List users
	users, err := store.ListUsers(ctx, 10, 0)
	require.NoError(t, err)
	assert.Len(t, users, 1)

	// Delete user
	err = store.DeleteUser(ctx, "test-user")
	require.NoError(t, err)

	_, err = store.GetUser(ctx, "test-user")
	assert.Error(t, err)
}

func TestSQLRBACStore_RoleOperations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()

	// Create user
	user := &User{
		ID:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Assign role
	err = store.AssignRole(ctx, "test-user", RoleOperator)
	require.NoError(t, err)

	// Get roles
	roles, err := store.GetUserRoles(ctx, "test-user")
	require.NoError(t, err)
	assert.Contains(t, roles, RoleOperator)

	// Assign another role
	err = store.AssignRole(ctx, "test-user", RoleViewer)
	require.NoError(t, err)

	roles, err = store.GetUserRoles(ctx, "test-user")
	require.NoError(t, err)
	assert.Len(t, roles, 2)

	// Revoke role
	err = store.RevokeRole(ctx, "test-user", RoleOperator)
	require.NoError(t, err)

	roles, err = store.GetUserRoles(ctx, "test-user")
	require.NoError(t, err)
	assert.Len(t, roles, 1)
	assert.Contains(t, roles, RoleViewer)
}

func TestSQLRBACStore_PermissionOperations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()

	// Create user
	user := &User{
		ID:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Grant permission
	err = store.GrantPermission(ctx, "test-user", PermissionDeviceView)
	require.NoError(t, err)

	// Get permissions
	perms, err := store.GetUserPermissions(ctx, "test-user")
	require.NoError(t, err)
	assert.Contains(t, perms, PermissionDeviceView)

	// Grant another permission
	err = store.GrantPermission(ctx, "test-user", PermissionUpdateView)
	require.NoError(t, err)

	perms, err = store.GetUserPermissions(ctx, "test-user")
	require.NoError(t, err)
	assert.Len(t, perms, 2)

	// Revoke permission
	err = store.RevokePermission(ctx, "test-user", PermissionDeviceView)
	require.NoError(t, err)

	perms, err = store.GetUserPermissions(ctx, "test-user")
	require.NoError(t, err)
	assert.Len(t, perms, 1)
	assert.Contains(t, perms, PermissionUpdateView)
}

func TestSQLRBACStore_PolicyOperations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()

	// Create policy
	policy := &Policy{
		ID:          "test-policy",
		Name:        "Test Policy",
		Description: "A test policy",
		Resource:    "/api/devices",
		Actions:     []string{"read", "write"},
		Effect:      PolicyEffectAllow,
		Conditions: []Condition{
			{
				Type:     "role",
				Operator: "equals",
				Value:    "operator",
			},
		},
		Priority: 10,
	}

	err = store.CreatePolicy(ctx, policy)
	require.NoError(t, err)

	// Get policy
	retrievedPolicy, err := store.GetPolicy(ctx, "test-policy")
	require.NoError(t, err)
	assert.Equal(t, policy.ID, retrievedPolicy.ID)
	assert.Equal(t, policy.Name, retrievedPolicy.Name)
	assert.Equal(t, policy.Resource, retrievedPolicy.Resource)
	assert.Equal(t, policy.Actions, retrievedPolicy.Actions)
	assert.Equal(t, policy.Effect, retrievedPolicy.Effect)

	// Update policy
	policy.Description = "Updated description"
	policy.Priority = 20
	err = store.UpdatePolicy(ctx, policy)
	require.NoError(t, err)

	retrievedPolicy, err = store.GetPolicy(ctx, "test-policy")
	require.NoError(t, err)
	assert.Equal(t, "Updated description", retrievedPolicy.Description)
	assert.Equal(t, 20, retrievedPolicy.Priority)

	// List policies
	policies, err := store.ListPolicies(ctx, 10, 0)
	require.NoError(t, err)
	assert.Len(t, policies, 1)

	// Get policies for resource
	resourcePolicies, err := store.GetPoliciesForResource(ctx, "/api/devices")
	require.NoError(t, err)
	assert.Len(t, resourcePolicies, 1)

	// Delete policy
	err = store.DeletePolicy(ctx, "test-policy")
	require.NoError(t, err)

	_, err = store.GetPolicy(ctx, "test-policy")
	assert.Error(t, err)
}

func TestRBACManager_ConditionEvaluation(t *testing.T) {
	manager := NewRBACManager()

	// Create user with operator role
	user := &User{
		ID:    "operator1",
		Roles: []Role{RoleOperator},
	}
	manager.CreateUser(user)

	// Test role condition evaluation
	condition := Condition{
		Type:     "role",
		Operator: "equals",
		Value:    "operator",
	}
	result := manager.evaluateRoleCondition(user, condition)
	assert.True(t, result)

	// Test negative role condition
	condition.Value = "admin"
	result = manager.evaluateRoleCondition(user, condition)
	assert.False(t, result)

	// Test not_equals operator
	condition.Operator = "not_equals"
	result = manager.evaluateRoleCondition(user, condition)
	assert.True(t, result)
}
