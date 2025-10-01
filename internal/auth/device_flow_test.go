package auth

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceFlow_CreateDeviceAuth(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	df := NewDeviceFlow(db)

	tests := []struct {
		name      string
		clientID  string
		scope     string
		wantErr   bool
		setupMock func()
	}{
		{
			name:     "successful device auth creation",
			clientID: "fleetctl",
			scope:    "api",
			wantErr:  false,
			setupMock: func() {
				mock.ExpectExec("INSERT INTO device_auth_request").
					WithArgs(
						sqlmock.AnyArg(), // id
						sqlmock.AnyArg(), // device_code
						sqlmock.AnyArg(), // user_code
						sqlmock.AnyArg(), // verification_url
						sqlmock.AnyArg(), // expires_at
						5,                // interval_seconds
						"fleetctl",       // client_id
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
		},
		{
			name:     "database error",
			clientID: "fleetctl",
			scope:    "api",
			wantErr:  true,
			setupMock: func() {
				mock.ExpectExec("INSERT INTO device_auth_request").
					WillReturnError(sql.ErrConnDone)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			resp, err := df.CreateDeviceAuth(tt.clientID, tt.scope)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.DeviceCode)
				assert.NotEmpty(t, resp.UserCode)
				assert.Equal(t, "https://localhost:3000/auth/device", resp.VerificationURL)
				assert.Equal(t, 900, resp.ExpiresIn)
				assert.Equal(t, 5, resp.Interval)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDeviceFlow_VerifyUserCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	df := NewDeviceFlow(db)

	tests := []struct {
		name      string
		userCode  string
		wantValid bool
		wantErr   bool
		setupMock func()
	}{
		{
			name:      "valid user code",
			userCode:  "ABCD-1234",
			wantValid: true,
			wantErr:   false,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "device_code", "user_code", "verification_url", "expires_at", "interval_seconds", "client_id", "client_name", "user_id", "approved_at"}).
					AddRow("test-id", "device-code", "ABCD1234", "https://localhost:3000/auth/device", time.Now().Add(10*time.Minute), 5, "fleetctl", "Fleet CLI", nil, nil)
				mock.ExpectQuery("SELECT id, device_code, user_code, verification_url, expires_at, interval_seconds, client_id, client_name, user_id, approved_at FROM device_auth_request WHERE").
					WithArgs("ABCD1234"). // Note: dashes are removed
					WillReturnRows(rows)
			},
		},
		{
			name:      "expired code",
			userCode:  "ABCD-1234",
			wantValid: false,
			wantErr:   false,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "device_code", "user_code", "verification_url", "expires_at", "interval_seconds", "client_id", "client_name", "user_id", "approved_at"}).
					AddRow("test-id", "device-code", "ABCD1234", "https://localhost:3000/auth/device", time.Now().Add(-10*time.Minute), 5, "fleetctl", "Fleet CLI", nil, nil)
				mock.ExpectQuery("SELECT id, device_code, user_code, verification_url, expires_at, interval_seconds, client_id, client_name, user_id, approved_at FROM device_auth_request WHERE").
					WithArgs("ABCD1234").
					WillReturnRows(rows)
			},
		},
		{
			name:      "already approved code",
			userCode:  "ABCD-1234",
			wantValid: false,
			wantErr:   false,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "device_code", "user_code", "verification_url", "expires_at", "interval_seconds", "client_id", "client_name", "user_id", "approved_at"}).
					AddRow("test-id", "device-code", "ABCD1234", "https://localhost:3000/auth/device", time.Now().Add(10*time.Minute), 5, "fleetctl", "Fleet CLI", nil, time.Now())
				mock.ExpectQuery("SELECT id, device_code, user_code, verification_url, expires_at, interval_seconds, client_id, client_name, user_id, approved_at FROM device_auth_request WHERE").
					WithArgs("ABCD1234").
					WillReturnRows(rows)
			},
		},
		{
			name:      "code not found",
			userCode:  "XXXX-XXXX",
			wantValid: false,
			wantErr:   true, // Should error when code not found
			setupMock: func() {
				mock.ExpectQuery("SELECT id, device_code, user_code, verification_url, expires_at, interval_seconds, client_id, client_name, user_id, approved_at FROM device_auth_request WHERE").
					WithArgs("XXXXXXXX").
					WillReturnError(sql.ErrNoRows)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			authReq, err := df.VerifyUserCode(tt.userCode)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, authReq)
			} else {
				assert.NoError(t, err)
				if tt.wantValid {
					assert.NotNil(t, authReq)
					assert.Equal(t, "ABCD1234", authReq.UserCode)
				} else {
					// For invalid cases (expired, already approved), we still get the request
					// but can check the state
					assert.NotNil(t, authReq)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDeviceFlow_ApproveDeviceAuth(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	df := NewDeviceFlow(db)

	tests := []struct {
		name      string
		userCode  string
		userID    string
		wantErr   bool
		setupMock func()
	}{
		{
			name:     "successful approval",
			userCode: "ABCD-1234",
			userID:    "user-123",
			wantErr:  false,
			setupMock: func() {
				mock.ExpectExec("UPDATE device_auth_request").
					WithArgs("user-123", "ABCD1234").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name:     "code not found",
			userCode: "XXXX-XXXX",
			userID:    "user-123",
			wantErr:  true,
			setupMock: func() {
				mock.ExpectExec("UPDATE device_auth_request").
					WithArgs("user-123", "XXXXXXXX").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := df.ApproveDeviceAuth(tt.userCode, tt.userID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDeviceFlow_ExchangeDeviceCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	df := NewDeviceFlow(db)

	tests := []struct {
		name       string
		deviceCode string
		clientID   string
		want       *TokenResponse
		wantErr    bool
		setupMock  func()
	}{
		{
			name:       "successful exchange",
			deviceCode: "test-device-code",
			clientID:   "fleetctl",
			want: &TokenResponse{
				TokenType: "Bearer",
				ExpiresIn: 86400,
			},
			wantErr: false,
			setupMock: func() {
				// Check if approved
				rows := sqlmock.NewRows([]string{"user_id", "approved_at", "expires_at"}).
					AddRow("user-123", time.Now(), time.Now().Add(10*time.Minute))
				mock.ExpectQuery("SELECT user_id, approved_at, expires_at FROM device_auth_request").
					WithArgs("test-device-code", "fleetctl").
					WillReturnRows(rows)

				// Insert access token
				mock.ExpectExec("INSERT INTO access_token").
					WithArgs(
						sqlmock.AnyArg(),      // id
						sqlmock.AnyArg(),      // token
						"user-123",            // user_id
						"test-device-code",    // device_auth_id
						sqlmock.AnyArg(),      // expires_at
						"fleetctl",            // client_id
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
		},
		{
			name:       "pending authorization",
			deviceCode: "test-device-code",
			clientID:   "fleetctl",
			want:       nil,
			wantErr:    true,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"user_id", "approved_at", "expires_at"}).
					AddRow(nil, nil, time.Now().Add(10*time.Minute))
				mock.ExpectQuery("SELECT user_id, approved_at, expires_at FROM device_auth_request").
					WithArgs("test-device-code", "fleetctl").
					WillReturnRows(rows)
			},
		},
		{
			name:       "expired code",
			deviceCode: "test-device-code",
			clientID:   "fleetctl",
			want:       nil,
			wantErr:    true,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"user_id", "approved_at", "expires_at"}).
					AddRow("user-123", time.Now(), time.Now().Add(-10*time.Minute))
				mock.ExpectQuery("SELECT user_id, approved_at, expires_at FROM device_auth_request").
					WithArgs("test-device-code", "fleetctl").
					WillReturnRows(rows)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			resp, err := df.ExchangeDeviceCode(tt.deviceCode, tt.clientID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.AccessToken)
				assert.Equal(t, tt.want.TokenType, resp.TokenType)
				assert.Equal(t, tt.want.ExpiresIn, resp.ExpiresIn)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGenerateUserCode(t *testing.T) {
	// Test that user codes are generated correctly
	codes := make(map[string]bool)

	for i := 0; i < 100; i++ {
		code := generateUserCode()

		// Check format
		assert.Len(t, code, 8, "User code should be 8 characters")
		assert.Regexp(t, "^[A-Z0-9]{8}$", code, "User code should only contain allowed characters")

		// Check uniqueness (probabilistic)
		assert.False(t, codes[code], "User code should be unique")
		codes[code] = true
	}
}

func TestGenerateDeviceCode(t *testing.T) {
	// Test that device codes are generated correctly
	codes := make(map[string]bool)

	for i := 0; i < 100; i++ {
		code := generateDeviceCode()

		// Check format
		assert.Len(t, code, 43, "Device code should be 43 characters (base64 without padding)")

		// Check uniqueness
		assert.False(t, codes[code], "Device code should be unique")
		codes[code] = true
	}
}