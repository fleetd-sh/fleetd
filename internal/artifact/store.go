package artifact

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"fleetd.sh/internal/security"
	"log/slog"
)

// Store manages binary artifacts for deployments
type Store struct {
	basePath   string
	db         *sql.DB
	signer     *security.Signer
	maxSize    int64 // Maximum artifact size in bytes
	cdnURL     string // Optional CDN URL prefix
}

// Artifact represents a stored artifact
type Artifact struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Version     string    `json:"version" db:"version"`
	Type        string    `json:"type" db:"type"` // binary, config, script
	Size        int64     `json:"size" db:"size"`
	Checksum    string    `json:"checksum" db:"checksum"`
	Signature   string    `json:"signature" db:"signature"`
	URL         string    `json:"url" db:"url"`
	Metadata    string    `json:"metadata" db:"metadata"` // JSON metadata
	UploadedAt  time.Time `json:"uploaded_at" db:"uploaded_at"`
	UploadedBy  string    `json:"uploaded_by" db:"uploaded_by"`
}

// NewStore creates a new artifact store
func NewStore(basePath string, db *sql.DB, signer *security.Signer) *Store {
	return &Store{
		basePath: basePath,
		db:       db,
		signer:   signer,
		maxSize:  1 << 30, // 1GB default
	}
}

// SetCDNURL sets the CDN URL prefix for artifact URLs
func (s *Store) SetCDNURL(cdnURL string) {
	s.cdnURL = cdnURL
}

// SetMaxSize sets the maximum allowed artifact size
func (s *Store) SetMaxSize(maxSize int64) {
	s.maxSize = maxSize
}

// StoreArtifact stores a new artifact
func (s *Store) StoreArtifact(ctx context.Context, reader io.Reader, meta *ArtifactMetadata) (*Artifact, error) {
	// Generate artifact ID
	artifactID := fmt.Sprintf("artifact-%s-%d", meta.Version, time.Now().Unix())

	// Create artifact directory
	artifactDir := filepath.Join(s.basePath, meta.Type, meta.Name, meta.Version)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Determine file extension based on type
	ext := ".tar.gz"
	switch meta.Type {
	case "binary":
		ext = getFileExtension(meta.Name)
	case "config":
		ext = ".json"
	case "script":
		ext = ".sh"
	}

	fileName := fmt.Sprintf("%s-%s%s", meta.Name, meta.Version, ext)
	filePath := filepath.Join(artifactDir, fileName)

	// Create temporary file for writing
	tempFile, err := os.CreateTemp(artifactDir, "temp-*.artifact")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	// Calculate checksum while writing
	hasher := sha256.New()
	multiWriter := io.MultiWriter(tempFile, hasher)

	size, err := io.CopyN(multiWriter, reader, s.maxSize+1)
	if err != nil && err != io.EOF {
		tempFile.Close()
		return nil, fmt.Errorf("failed to write artifact: %w", err)
	}
	tempFile.Close()

	if size > s.maxSize {
		return nil, fmt.Errorf("artifact exceeds maximum size of %d bytes", s.maxSize)
	}

	// Calculate checksum
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Sign the artifact
	signature := ""
	if s.signer != nil {
		sig, err := s.signer.SignFile(tempPath)
		if err != nil {
			return nil, fmt.Errorf("failed to sign artifact: %w", err)
		}
		signature = sig
	}

	// Move temp file to final location
	if err := os.Rename(tempPath, filePath); err != nil {
		return nil, fmt.Errorf("failed to move artifact: %w", err)
	}

	// Generate URL
	url := s.generateURL(meta.Type, meta.Name, meta.Version, fileName)

	// Store metadata in database
	artifact := &Artifact{
		ID:         artifactID,
		Name:       meta.Name,
		Version:    meta.Version,
		Type:       meta.Type,
		Size:       size,
		Checksum:   checksum,
		Signature:  signature,
		URL:        url,
		Metadata:   meta.MetadataJSON,
		UploadedAt: time.Now(),
		UploadedBy: meta.UploadedBy,
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO artifacts (
			id, name, version, type, size, checksum, signature,
			url, metadata, uploaded_at, uploaded_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		artifact.ID, artifact.Name, artifact.Version, artifact.Type,
		artifact.Size, artifact.Checksum, artifact.Signature,
		artifact.URL, artifact.Metadata, artifact.UploadedAt, artifact.UploadedBy)

	if err != nil {
		os.Remove(filePath) // Clean up file if DB insert fails
		return nil, fmt.Errorf("failed to store metadata: %w", err)
	}

	slog.Info("Artifact stored successfully",
		"id", artifactID,
		"name", meta.Name,
		"version", meta.Version,
		"size", size,
		"checksum", checksum)

	return artifact, nil
}

// GetArtifact retrieves artifact metadata
func (s *Store) GetArtifact(ctx context.Context, artifactID string) (*Artifact, error) {
	var artifact Artifact
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, version, type, size, checksum, signature,
		       url, metadata, uploaded_at, uploaded_by
		FROM artifacts
		WHERE id = ?`,
		artifactID).Scan(
		&artifact.ID, &artifact.Name, &artifact.Version, &artifact.Type,
		&artifact.Size, &artifact.Checksum, &artifact.Signature,
		&artifact.URL, &artifact.Metadata, &artifact.UploadedAt, &artifact.UploadedBy)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("artifact not found")
	}
	if err != nil {
		return nil, err
	}

	return &artifact, nil
}

// GetArtifactByVersion retrieves artifact by name and version
func (s *Store) GetArtifactByVersion(ctx context.Context, name, version string) (*Artifact, error) {
	var artifact Artifact
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, version, type, size, checksum, signature,
		       url, metadata, uploaded_at, uploaded_by
		FROM artifacts
		WHERE name = ? AND version = ?
		ORDER BY uploaded_at DESC
		LIMIT 1`,
		name, version).Scan(
		&artifact.ID, &artifact.Name, &artifact.Version, &artifact.Type,
		&artifact.Size, &artifact.Checksum, &artifact.Signature,
		&artifact.URL, &artifact.Metadata, &artifact.UploadedAt, &artifact.UploadedBy)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("artifact not found")
	}
	if err != nil {
		return nil, err
	}

	return &artifact, nil
}

// ListArtifacts lists all artifacts with optional filtering
func (s *Store) ListArtifacts(ctx context.Context, filter *ArtifactFilter) ([]*Artifact, error) {
	query := `
		SELECT id, name, version, type, size, checksum, signature,
		       url, metadata, uploaded_at, uploaded_by
		FROM artifacts
		WHERE 1=1
	`
	args := []interface{}{}

	if filter != nil {
		if filter.Name != "" {
			query += " AND name = ?"
			args = append(args, filter.Name)
		}
		if filter.Type != "" {
			query += " AND type = ?"
			args = append(args, filter.Type)
		}
		if filter.MinVersion != "" {
			query += " AND version >= ?"
			args = append(args, filter.MinVersion)
		}
		if filter.MaxVersion != "" {
			query += " AND version <= ?"
			args = append(args, filter.MaxVersion)
		}
	}

	query += " ORDER BY uploaded_at DESC"
	if filter != nil && filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	artifacts := []*Artifact{}
	for rows.Next() {
		var a Artifact
		err := rows.Scan(
			&a.ID, &a.Name, &a.Version, &a.Type,
			&a.Size, &a.Checksum, &a.Signature,
			&a.URL, &a.Metadata, &a.UploadedAt, &a.UploadedBy)
		if err != nil {
			continue
		}
		artifacts = append(artifacts, &a)
	}

	return artifacts, nil
}

// VerifyArtifact verifies an artifact's integrity
func (s *Store) VerifyArtifact(ctx context.Context, artifactID string) error {
	artifact, err := s.GetArtifact(ctx, artifactID)
	if err != nil {
		return err
	}

	// Get file path from URL
	filePath := s.getFilePath(artifact)
	if filePath == "" {
		return fmt.Errorf("cannot determine file path")
	}

	// Verify file exists
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("artifact file not found: %w", err)
	}

	// Verify size
	if info.Size() != artifact.Size {
		return fmt.Errorf("size mismatch: expected %d, got %d", artifact.Size, info.Size())
	}

	// Verify checksum
	checksum, err := security.CalculateChecksum(filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	if checksum != artifact.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", artifact.Checksum, checksum)
	}

	// Verify signature if present
	if artifact.Signature != "" && s.signer != nil {
		if err := s.signer.VerifyFile(filePath, artifact.Signature); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	return nil
}

// DeleteArtifact deletes an artifact
func (s *Store) DeleteArtifact(ctx context.Context, artifactID string) error {
	artifact, err := s.GetArtifact(ctx, artifactID)
	if err != nil {
		return err
	}

	// Delete from database
	_, err = s.db.ExecContext(ctx, "DELETE FROM artifacts WHERE id = ?", artifactID)
	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	// Delete file
	filePath := s.getFilePath(artifact)
	if filePath != "" {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			slog.Warn("Failed to delete artifact file", "path", filePath, "error", err)
		}
	}

	slog.Info("Artifact deleted", "id", artifactID)
	return nil
}

// CleanupOldArtifacts removes artifacts older than the specified duration
func (s *Store) CleanupOldArtifacts(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)

	// Get artifacts to delete
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM artifacts
		WHERE uploaded_at < ?`,
		cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var artifactID string
		if err := rows.Scan(&artifactID); err != nil {
			continue
		}

		if err := s.DeleteArtifact(ctx, artifactID); err != nil {
			slog.Error("Failed to delete old artifact", "id", artifactID, "error", err)
			continue
		}
		count++
	}

	slog.Info("Cleaned up old artifacts", "count", count, "max_age", maxAge)
	return nil
}

// generateURL generates the download URL for an artifact
func (s *Store) generateURL(artifactType, name, version, fileName string) string {
	if s.cdnURL != "" {
		return fmt.Sprintf("%s/%s/%s/%s/%s", s.cdnURL, artifactType, name, version, fileName)
	}
	// Local file URL
	return fmt.Sprintf("file://%s/%s/%s/%s/%s", s.basePath, artifactType, name, version, fileName)
}

// getFilePath gets the local file path for an artifact
func (s *Store) getFilePath(artifact *Artifact) string {
	// Extract path from URL
	if artifact.URL[:7] == "file://" {
		return artifact.URL[7:]
	}
	// For CDN URLs, construct local path
	ext := ".tar.gz"
	switch artifact.Type {
	case "config":
		ext = ".json"
	case "script":
		ext = ".sh"
	}
	fileName := fmt.Sprintf("%s-%s%s", artifact.Name, artifact.Version, ext)
	return filepath.Join(s.basePath, artifact.Type, artifact.Name, artifact.Version, fileName)
}

// getFileExtension determines file extension based on artifact name
func getFileExtension(name string) string {
	// Check if name already has an extension
	if ext := filepath.Ext(name); ext != "" {
		return ""
	}
	// Default to tar.gz for binaries
	return ".tar.gz"
}

// ArtifactMetadata contains metadata for storing an artifact
type ArtifactMetadata struct {
	Name         string
	Version      string
	Type         string // binary, config, script
	MetadataJSON string // Additional metadata as JSON
	UploadedBy   string
}

// ArtifactFilter contains filter criteria for listing artifacts
type ArtifactFilter struct {
	Name       string
	Type       string
	MinVersion string
	MaxVersion string
	Limit      int
}