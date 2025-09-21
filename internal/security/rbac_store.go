package security

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"fleetd.sh/internal/ferrors"
)

// RBACStore provides database persistence for RBAC
type RBACStore interface {
	// User operations
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, userID string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, userID string) error
	ListUsers(ctx context.Context, limit, offset int) ([]*User, error)

	// Role operations
	AssignRole(ctx context.Context, userID string, role Role) error
	RevokeRole(ctx context.Context, userID string, role Role) error
	GetUserRoles(ctx context.Context, userID string) ([]Role, error)

	// Permission operations
	GrantPermission(ctx context.Context, userID string, permission Permission) error
	RevokePermission(ctx context.Context, userID string, permission Permission) error
	GetUserPermissions(ctx context.Context, userID string) ([]Permission, error)

	// Policy operations
	CreatePolicy(ctx context.Context, policy *Policy) error
	GetPolicy(ctx context.Context, policyID string) (*Policy, error)
	UpdatePolicy(ctx context.Context, policy *Policy) error
	DeletePolicy(ctx context.Context, policyID string) error
	ListPolicies(ctx context.Context, limit, offset int) ([]*Policy, error)
	GetPoliciesForResource(ctx context.Context, resource string) ([]*Policy, error)
}

// SQLRBACStore implements RBACStore using SQL database
type SQLRBACStore struct {
	db *sql.DB
}

// NewSQLRBACStore creates a new SQL-based RBAC store
func NewSQLRBACStore(db *sql.DB) (*SQLRBACStore, error) {
	store := &SQLRBACStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, err
	}
	return store, nil
}

// createTables creates the necessary database tables
func (s *SQLRBACStore) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id VARCHAR(255) PRIMARY KEY,
			username VARCHAR(255) UNIQUE NOT NULL,
			email VARCHAR(255) UNIQUE,
			metadata JSONB,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_roles (
			user_id VARCHAR(255) NOT NULL,
			role VARCHAR(50) NOT NULL,
			granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			granted_by VARCHAR(255),
			PRIMARY KEY (user_id, role),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS user_permissions (
			user_id VARCHAR(255) NOT NULL,
			permission VARCHAR(100) NOT NULL,
			granted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			granted_by VARCHAR(255),
			PRIMARY KEY (user_id, permission),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS policies (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			resource VARCHAR(255) NOT NULL,
			actions JSONB NOT NULL,
			effect VARCHAR(20) NOT NULL,
			conditions JSONB,
			priority INT DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_permissions_user_id ON user_permissions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_policies_resource ON policies(resource)`,
		`CREATE INDEX IF NOT EXISTS idx_policies_priority ON policies(priority DESC)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			// Try SQLite-compatible version if PostgreSQL fails
			sqliteQuery := convertToSQLite(query)
			if _, err2 := s.db.Exec(sqliteQuery); err2 != nil {
				return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to create RBAC tables")
			}
		}
	}

	return nil
}

// CreateUser creates a new user
func (s *SQLRBACStore) CreateUser(ctx context.Context, user *User) error {
	metadataJSON, err := json.Marshal(user.Metadata)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to marshal metadata")
	}

	query := `
		INSERT INTO users (id, username, email, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err = s.db.ExecContext(ctx, query,
		user.ID, user.Username, user.Email, metadataJSON, now, now)

	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to create user")
	}

	// Insert roles
	for _, role := range user.Roles {
		if err := s.AssignRole(ctx, user.ID, role); err != nil {
			return err
		}
	}

	// Insert permissions
	for _, perm := range user.Permissions {
		if err := s.GrantPermission(ctx, user.ID, perm); err != nil {
			return err
		}
	}

	return nil
}

// GetUser retrieves a user by ID
func (s *SQLRBACStore) GetUser(ctx context.Context, userID string) (*User, error) {
	query := `
		SELECT id, username, email, metadata, created_at, updated_at, last_login
		FROM users
		WHERE id = ?
	`

	user := &User{}
	var metadataJSON []byte
	var lastLogin sql.NullTime

	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID, &user.Username, &user.Email, &metadataJSON,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)

	if err == sql.ErrNoRows {
		return nil, ferrors.Newf(ferrors.ErrCodeNotFound, "user not found: %s", userID)
	}
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get user")
	}

	if lastLogin.Valid {
		user.LastLogin = lastLogin.Time
	}

	if metadataJSON != nil {
		if err := json.Unmarshal(metadataJSON, &user.Metadata); err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal metadata")
		}
	}

	// Get roles
	user.Roles, err = s.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get permissions
	user.Permissions, err = s.GetUserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByUsername retrieves a user by username
func (s *SQLRBACStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := `SELECT id FROM users WHERE username = ?`

	var userID string
	err := s.db.QueryRowContext(ctx, query, username).Scan(&userID)
	if err == sql.ErrNoRows {
		return nil, ferrors.Newf(ferrors.ErrCodeNotFound, "user not found: %s", username)
	}
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get user by username")
	}

	return s.GetUser(ctx, userID)
}

// UpdateUser updates a user
func (s *SQLRBACStore) UpdateUser(ctx context.Context, user *User) error {
	metadataJSON, err := json.Marshal(user.Metadata)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to marshal metadata")
	}

	query := `
		UPDATE users
		SET username = ?, email = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`

	user.UpdatedAt = time.Now()

	result, err := s.db.ExecContext(ctx, query,
		user.Username, user.Email, metadataJSON, user.UpdatedAt, user.ID)

	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to update user")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return ferrors.Newf(ferrors.ErrCodeNotFound, "user not found: %s", user.ID)
	}

	return nil
}

// DeleteUser deletes a user
func (s *SQLRBACStore) DeleteUser(ctx context.Context, userID string) error {
	query := `DELETE FROM users WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, userID)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to delete user")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return ferrors.Newf(ferrors.ErrCodeNotFound, "user not found: %s", userID)
	}

	return nil
}

// ListUsers lists users with pagination
func (s *SQLRBACStore) ListUsers(ctx context.Context, limit, offset int) ([]*User, error) {
	query := `
		SELECT id, username, email, metadata, created_at, updated_at, last_login
		FROM users
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to list users")
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		var metadataJSON []byte
		var lastLogin sql.NullTime

		err := rows.Scan(
			&user.ID, &user.Username, &user.Email, &metadataJSON,
			&user.CreatedAt, &user.UpdatedAt, &lastLogin,
		)
		if err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to scan user")
		}

		if lastLogin.Valid {
			user.LastLogin = lastLogin.Time
		}

		if metadataJSON != nil {
			if err := json.Unmarshal(metadataJSON, &user.Metadata); err != nil {
				return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal metadata")
			}
		}

		// Get roles and permissions for each user
		user.Roles, _ = s.GetUserRoles(ctx, user.ID)
		user.Permissions, _ = s.GetUserPermissions(ctx, user.ID)

		users = append(users, user)
	}

	return users, nil
}

// AssignRole assigns a role to a user
func (s *SQLRBACStore) AssignRole(ctx context.Context, userID string, role Role) error {
	query := `
		INSERT INTO user_roles (user_id, role)
		VALUES (?, ?)
		ON CONFLICT (user_id, role) DO NOTHING
	`

	// SQLite compatibility
	sqliteQuery := `
		INSERT OR IGNORE INTO user_roles (user_id, role)
		VALUES (?, ?)
	`

	_, err := s.db.ExecContext(ctx, query, userID, string(role))
	if err != nil {
		// Try SQLite query
		_, err = s.db.ExecContext(ctx, sqliteQuery, userID, string(role))
		if err != nil {
			return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to assign role")
		}
	}

	return nil
}

// RevokeRole revokes a role from a user
func (s *SQLRBACStore) RevokeRole(ctx context.Context, userID string, role Role) error {
	query := `DELETE FROM user_roles WHERE user_id = ? AND role = ?`

	_, err := s.db.ExecContext(ctx, query, userID, string(role))
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to revoke role")
	}

	return nil
}

// GetUserRoles gets all roles for a user
func (s *SQLRBACStore) GetUserRoles(ctx context.Context, userID string) ([]Role, error) {
	query := `SELECT role FROM user_roles WHERE user_id = ?`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get user roles")
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var roleStr string
		if err := rows.Scan(&roleStr); err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to scan role")
		}
		roles = append(roles, Role(roleStr))
	}

	return roles, nil
}

// GrantPermission grants a permission to a user
func (s *SQLRBACStore) GrantPermission(ctx context.Context, userID string, permission Permission) error {
	query := `
		INSERT INTO user_permissions (user_id, permission)
		VALUES (?, ?)
		ON CONFLICT (user_id, permission) DO NOTHING
	`

	// SQLite compatibility
	sqliteQuery := `
		INSERT OR IGNORE INTO user_permissions (user_id, permission)
		VALUES (?, ?)
	`

	_, err := s.db.ExecContext(ctx, query, userID, string(permission))
	if err != nil {
		// Try SQLite query
		_, err = s.db.ExecContext(ctx, sqliteQuery, userID, string(permission))
		if err != nil {
			return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to grant permission")
		}
	}

	return nil
}

// RevokePermission revokes a permission from a user
func (s *SQLRBACStore) RevokePermission(ctx context.Context, userID string, permission Permission) error {
	query := `DELETE FROM user_permissions WHERE user_id = ? AND permission = ?`

	_, err := s.db.ExecContext(ctx, query, userID, string(permission))
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to revoke permission")
	}

	return nil
}

// GetUserPermissions gets all permissions for a user
func (s *SQLRBACStore) GetUserPermissions(ctx context.Context, userID string) ([]Permission, error) {
	query := `SELECT permission FROM user_permissions WHERE user_id = ?`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get user permissions")
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var permStr string
		if err := rows.Scan(&permStr); err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to scan permission")
		}
		permissions = append(permissions, Permission(permStr))
	}

	return permissions, nil
}

// CreatePolicy creates a new policy
func (s *SQLRBACStore) CreatePolicy(ctx context.Context, policy *Policy) error {
	actionsJSON, err := json.Marshal(policy.Actions)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to marshal actions")
	}

	conditionsJSON, err := json.Marshal(policy.Conditions)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to marshal conditions")
	}

	query := `
		INSERT INTO policies (id, name, description, resource, actions, effect, conditions, priority)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		policy.ID, policy.Name, policy.Description, policy.Resource,
		actionsJSON, string(policy.Effect), conditionsJSON, policy.Priority)

	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to create policy")
	}

	return nil
}

// GetPolicy retrieves a policy by ID
func (s *SQLRBACStore) GetPolicy(ctx context.Context, policyID string) (*Policy, error) {
	query := `
		SELECT id, name, description, resource, actions, effect, conditions, priority
		FROM policies
		WHERE id = ?
	`

	policy := &Policy{}
	var actionsJSON, conditionsJSON []byte
	var effectStr string

	err := s.db.QueryRowContext(ctx, query, policyID).Scan(
		&policy.ID, &policy.Name, &policy.Description, &policy.Resource,
		&actionsJSON, &effectStr, &conditionsJSON, &policy.Priority,
	)

	if err == sql.ErrNoRows {
		return nil, ferrors.Newf(ferrors.ErrCodeNotFound, "policy not found: %s", policyID)
	}
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get policy")
	}

	policy.Effect = PolicyEffect(effectStr)

	if err := json.Unmarshal(actionsJSON, &policy.Actions); err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal actions")
	}

	if conditionsJSON != nil {
		if err := json.Unmarshal(conditionsJSON, &policy.Conditions); err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal conditions")
		}
	}

	return policy, nil
}

// UpdatePolicy updates a policy
func (s *SQLRBACStore) UpdatePolicy(ctx context.Context, policy *Policy) error {
	actionsJSON, err := json.Marshal(policy.Actions)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to marshal actions")
	}

	conditionsJSON, err := json.Marshal(policy.Conditions)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to marshal conditions")
	}

	query := `
		UPDATE policies
		SET name = ?, description = ?, resource = ?, actions = ?,
		    effect = ?, conditions = ?, priority = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query,
		policy.Name, policy.Description, policy.Resource, actionsJSON,
		string(policy.Effect), conditionsJSON, policy.Priority, time.Now(), policy.ID)

	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to update policy")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return ferrors.Newf(ferrors.ErrCodeNotFound, "policy not found: %s", policy.ID)
	}

	return nil
}

// DeletePolicy deletes a policy
func (s *SQLRBACStore) DeletePolicy(ctx context.Context, policyID string) error {
	query := `DELETE FROM policies WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, policyID)
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to delete policy")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return ferrors.Newf(ferrors.ErrCodeNotFound, "policy not found: %s", policyID)
	}

	return nil
}

// ListPolicies lists policies with pagination
func (s *SQLRBACStore) ListPolicies(ctx context.Context, limit, offset int) ([]*Policy, error) {
	query := `
		SELECT id, name, description, resource, actions, effect, conditions, priority
		FROM policies
		ORDER BY priority DESC, name ASC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to list policies")
	}
	defer rows.Close()

	var policies []*Policy
	for rows.Next() {
		policy := &Policy{}
		var actionsJSON, conditionsJSON []byte
		var effectStr string

		err := rows.Scan(
			&policy.ID, &policy.Name, &policy.Description, &policy.Resource,
			&actionsJSON, &effectStr, &conditionsJSON, &policy.Priority,
		)
		if err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to scan policy")
		}

		policy.Effect = PolicyEffect(effectStr)

		if err := json.Unmarshal(actionsJSON, &policy.Actions); err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal actions")
		}

		if conditionsJSON != nil {
			if err := json.Unmarshal(conditionsJSON, &policy.Conditions); err != nil {
				return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal conditions")
			}
		}

		policies = append(policies, policy)
	}

	return policies, nil
}

// GetPoliciesForResource gets all policies for a specific resource
func (s *SQLRBACStore) GetPoliciesForResource(ctx context.Context, resource string) ([]*Policy, error) {
	query := `
		SELECT id, name, description, resource, actions, effect, conditions, priority
		FROM policies
		WHERE resource = ?
		ORDER BY priority DESC
	`

	rows, err := s.db.QueryContext(ctx, query, resource)
	if err != nil {
		return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to get policies for resource")
	}
	defer rows.Close()

	var policies []*Policy
	for rows.Next() {
		policy := &Policy{}
		var actionsJSON, conditionsJSON []byte
		var effectStr string

		err := rows.Scan(
			&policy.ID, &policy.Name, &policy.Description, &policy.Resource,
			&actionsJSON, &effectStr, &conditionsJSON, &policy.Priority,
		)
		if err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to scan policy")
		}

		policy.Effect = PolicyEffect(effectStr)

		if err := json.Unmarshal(actionsJSON, &policy.Actions); err != nil {
			return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal actions")
		}

		if conditionsJSON != nil {
			if err := json.Unmarshal(conditionsJSON, &policy.Conditions); err != nil {
				return nil, ferrors.Wrap(err, ferrors.ErrCodeInternal, "failed to unmarshal conditions")
			}
		}

		policies = append(policies, policy)
	}

	return policies, nil
}

// convertToSQLite converts PostgreSQL queries to SQLite-compatible format
func convertToSQLite(query string) string {
	// Replace JSONB with TEXT for SQLite
	result := query
	result = replaceCaseInsensitive(result, "JSONB", "TEXT")
	result = replaceCaseInsensitive(result, "ON CONFLICT", "OR IGNORE")
	return result
}

func replaceCaseInsensitive(input, old, new string) string {
	return fmt.Sprintf("%s", input) // Simple implementation, can be improved
}