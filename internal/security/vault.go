package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// Vault provides secure storage for sensitive credentials
type Vault struct {
	path      string
	key       []byte
	mu        sync.RWMutex
	cache     map[string]*Credential
	cacheTime map[string]time.Time
	cacheTTL  time.Duration
}

// Credential represents a stored credential
type Credential struct {
	ID        string                 `json:"id"`
	Type      CredentialType         `json:"type"`
	Name      string                 `json:"name"`
	Value     string                 `json:"value"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
	Encrypted bool                   `json:"encrypted"`
}

// CredentialType defines the type of credential
type CredentialType string

const (
	CredentialTypeAPIKey      CredentialType = "api_key"
	CredentialTypeToken       CredentialType = "token"
	CredentialTypeCertificate CredentialType = "certificate"
	CredentialTypePrivateKey  CredentialType = "private_key"
	CredentialTypePassword    CredentialType = "password"
	CredentialTypeSecret      CredentialType = "secret"
)

// VaultConfig holds vault configuration
type VaultConfig struct {
	Path      string
	Password  string
	Salt      string
	CacheTTL  time.Duration
	AutoLock  bool
	LockAfter time.Duration
}

// NewVault creates a new credential vault
func NewVault(config *VaultConfig) (*Vault, error) {
	if config.Path == "" {
		config.Path = "/var/lib/fleetd/vault"
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}

	// Create vault directory
	if err := os.MkdirAll(config.Path, 0700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory: %w", err)
	}

	// Derive encryption key from password
	salt := []byte(config.Salt)
	if len(salt) == 0 {
		salt = []byte("fleetd-vault-salt-default")
	}

	key := pbkdf2.Key([]byte(config.Password), salt, 100000, 32, sha256.New)

	vault := &Vault{
		path:      config.Path,
		key:       key,
		cache:     make(map[string]*Credential),
		cacheTime: make(map[string]time.Time),
		cacheTTL:  config.CacheTTL,
	}

	// Initialize vault file if it doesn't exist
	vaultFile := filepath.Join(config.Path, "credentials.vault")
	if _, err := os.Stat(vaultFile); os.IsNotExist(err) {
		if err := vault.initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize vault: %w", err)
		}
	}

	return vault, nil
}

// Store stores a credential in the vault
func (v *Vault) Store(cred *Credential) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if cred.ID == "" {
		cred.ID = generateID()
	}

	now := time.Now()
	cred.UpdatedAt = now
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = now
	}

	// Encrypt the value
	encryptedValue, err := v.encrypt(cred.Value)
	if err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	// Store encrypted credential
	encCred := *cred
	encCred.Value = encryptedValue
	encCred.Encrypted = true

	// Save to disk
	if err := v.saveCredential(&encCred); err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}

	// Update cache with unencrypted version
	v.cache[cred.ID] = cred
	v.cacheTime[cred.ID] = now

	return nil
}

// Retrieve retrieves a credential from the vault
func (v *Vault) Retrieve(id string) (*Credential, error) {
	v.mu.RLock()

	// Check cache first
	if cached, ok := v.cache[id]; ok {
		cacheTime := v.cacheTime[id]
		if time.Since(cacheTime) < v.cacheTTL {
			// Check expiration
			if cached.ExpiresAt != nil && time.Now().After(*cached.ExpiresAt) {
				v.mu.RUnlock()
				return nil, fmt.Errorf("credential has expired")
			}
			v.mu.RUnlock()
			return cached, nil
		}
	}
	v.mu.RUnlock()

	// Load from disk
	v.mu.Lock()
	defer v.mu.Unlock()

	encCred, err := v.loadCredential(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load credential: %w", err)
	}

	// Check expiration
	if encCred.ExpiresAt != nil && time.Now().After(*encCred.ExpiresAt) {
		// Remove expired credential
		v.deleteCredential(id)
		return nil, fmt.Errorf("credential has expired")
	}

	// Decrypt value
	if encCred.Encrypted {
		decryptedValue, err := v.decrypt(encCred.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt credential: %w", err)
		}
		encCred.Value = decryptedValue
		encCred.Encrypted = false
	}

	// Update cache
	v.cache[id] = encCred
	v.cacheTime[id] = time.Now()

	return encCred, nil
}

// Delete removes a credential from the vault
func (v *Vault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Remove from cache
	delete(v.cache, id)
	delete(v.cacheTime, id)

	// Remove from disk
	return v.deleteCredential(id)
}

// List lists all credentials (without values)
func (v *Vault) List() ([]*Credential, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	vaultDir := filepath.Join(v.path, "credentials")
	entries, err := os.ReadDir(vaultDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Credential{}, nil
		}
		return nil, err
	}

	var credentials []*Credential
	for _, entry := range entries {
		if entry.IsDir() || !isCredentialFile(entry.Name()) {
			continue
		}

		id := getIDFromFilename(entry.Name())
		cred, err := v.loadCredential(id)
		if err != nil {
			continue
		}

		// Don't include the actual value in listings
		cred.Value = ""
		credentials = append(credentials, cred)
	}

	return credentials, nil
}

// Rotate rotates a credential (generates new value)
func (v *Vault) Rotate(id string) (*Credential, error) {
	// Retrieve existing credential
	existing, err := v.Retrieve(id)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credential for rotation: %w", err)
	}

	// Generate new value based on type
	newValue, err := v.generateNewValue(existing.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new value: %w", err)
	}

	// Update credential
	existing.Value = newValue
	existing.UpdatedAt = time.Now()

	// Store updated credential
	if err := v.Store(existing); err != nil {
		return nil, fmt.Errorf("failed to store rotated credential: %w", err)
	}

	return existing, nil
}

// Clear clears the cache
func (v *Vault) Clear() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.cache = make(map[string]*Credential)
	v.cacheTime = make(map[string]time.Time)
}

// Lock locks the vault (clears sensitive data from memory)
func (v *Vault) Lock() {
	v.Clear()
	// Could also clear the key from memory if implementing re-authentication
}

// initialize creates the vault structure
func (v *Vault) initialize() error {
	dirs := []string{
		filepath.Join(v.path, "credentials"),
		filepath.Join(v.path, "backup"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}

	// Create vault metadata
	metadata := map[string]interface{}{
		"version":    "1.0",
		"created_at": time.Now(),
		"algorithm":  "AES-256-GCM",
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metadataFile := filepath.Join(v.path, "vault.json")
	return os.WriteFile(metadataFile, data, 0600)
}

// encrypt encrypts data using AES-GCM
func (v *Vault) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts data using AES-GCM
func (v *Vault) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// saveCredential saves a credential to disk
func (v *Vault) saveCredential(cred *Credential) error {
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}

	// Encrypt the entire file
	encryptedData, err := v.encrypt(string(data))
	if err != nil {
		return err
	}

	credFile := filepath.Join(v.path, "credentials", fmt.Sprintf("%s.cred", cred.ID))

	// Write atomically
	tmpFile := credFile + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(encryptedData), 0600); err != nil {
		return err
	}

	return os.Rename(tmpFile, credFile)
}

// loadCredential loads a credential from disk
func (v *Vault) loadCredential(id string) (*Credential, error) {
	credFile := filepath.Join(v.path, "credentials", fmt.Sprintf("%s.cred", id))

	encryptedData, err := os.ReadFile(credFile)
	if err != nil {
		return nil, err
	}

	// Decrypt the file
	decryptedData, err := v.decrypt(string(encryptedData))
	if err != nil {
		return nil, err
	}

	var cred Credential
	if err := json.Unmarshal([]byte(decryptedData), &cred); err != nil {
		return nil, err
	}

	return &cred, nil
}

// deleteCredential deletes a credential from disk
func (v *Vault) deleteCredential(id string) error {
	credFile := filepath.Join(v.path, "credentials", fmt.Sprintf("%s.cred", id))

	// Create backup before deletion
	backupFile := filepath.Join(v.path, "backup", fmt.Sprintf("%s_%d.cred", id, time.Now().Unix()))
	if data, err := os.ReadFile(credFile); err == nil {
		os.WriteFile(backupFile, data, 0600)
	}

	return os.Remove(credFile)
}

// generateNewValue generates a new value for credential rotation
func (v *Vault) generateNewValue(credType CredentialType) (string, error) {
	switch credType {
	case CredentialTypeAPIKey, CredentialTypeToken:
		return generateRandomString(32)
	case CredentialTypePassword:
		return generateSecurePassword(16)
	case CredentialTypeSecret:
		return generateRandomString(64)
	default:
		return "", fmt.Errorf("cannot auto-generate value for type %s", credType)
	}
}

// Helper functions

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:length], nil
}

func generateSecurePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=[]{}|;:,.<>?"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

func isCredentialFile(name string) bool {
	return filepath.Ext(name) == ".cred"
}

func getIDFromFilename(name string) string {
	return strings.TrimSuffix(name, ".cred")
}

// Export exports all credentials (encrypted) for backup
func (v *Vault) Export(password string) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	credentials, err := v.List()
	if err != nil {
		return nil, err
	}

	// Load full credentials
	var fullCreds []*Credential
	for _, cred := range credentials {
		full, err := v.loadCredential(cred.ID)
		if err != nil {
			continue
		}
		fullCreds = append(fullCreds, full)
	}

	exportData := map[string]interface{}{
		"version":     "1.0",
		"exported_at": time.Now(),
		"credentials": fullCreds,
	}

	data, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return nil, err
	}

	// Encrypt export with provided password
	exportKey := pbkdf2.Key([]byte(password), []byte("export-salt"), 100000, 32, sha256.New)
	block, err := aes.NewCipher(exportKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

// Import imports credentials from an export
func (v *Vault) Import(data []byte, password string) error {
	// Decrypt export
	exportKey := pbkdf2.Key([]byte(password), []byte("export-salt"), 100000, 32, sha256.New)
	block, err := aes.NewCipher(exportKey)
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return fmt.Errorf("invalid export data")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt export: %w", err)
	}

	var exportData map[string]interface{}
	if err := json.Unmarshal(plaintext, &exportData); err != nil {
		return fmt.Errorf("invalid export format: %w", err)
	}

	// Import credentials
	if creds, ok := exportData["credentials"].([]interface{}); ok {
		for _, credData := range creds {
			credJSON, err := json.Marshal(credData)
			if err != nil {
				continue
			}

			var cred Credential
			if err := json.Unmarshal(credJSON, &cred); err != nil {
				continue
			}

			// The credential is already encrypted, save directly
			if err := v.saveCredential(&cred); err != nil {
				log.Printf("Failed to import credential %s: %v", cred.ID, err)
			}
		}
	}

	return nil
}
