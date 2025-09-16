package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	pb "fleetd.sh/gen/fleetd/v1"
	rpc "fleetd.sh/gen/fleetd/v1/fleetpbconnect"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type BinaryService struct {
	rpc.UnimplementedBinaryServiceHandler
	db          *sql.DB
	storagePath string
}

func NewBinaryService(db *sql.DB, storagePath string) (*BinaryService, error) {
	// Ensure storage directory exists
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %v", err)
	}
	return &BinaryService{db: db, storagePath: storagePath}, nil
}

func (s *BinaryService) UploadBinary(ctx context.Context, stream *connect.ClientStream[pb.UploadBinaryRequest]) (*connect.Response[pb.UploadBinaryResponse], error) {
	var (
		metadata   *pb.BinaryMetadata
		binaryFile *os.File
		hasher     = sha256.New()
		size       int64
		binaryID   = uuid.New().String()
		binaryPath string
	)

	defer func() {
		if binaryFile != nil {
			binaryFile.Close()
		}
	}()

	for {
		ok := stream.Receive()
		if !ok {
			break
		}

		req := stream.Msg()
		if req.Data == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("no data received"))
		}

		switch data := req.Data.(type) {
		case *pb.UploadBinaryRequest_Metadata:
			if metadata != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("metadata already received"))
			}
			metadata = data.Metadata
			binaryPath = filepath.Join(s.storagePath, binaryID)
			var err error
			binaryFile, err = os.Create(binaryPath)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create binary file: %v", err))
			}

		case *pb.UploadBinaryRequest_Chunk:
			if metadata == nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("metadata not received"))
			}
			if binaryFile == nil {
				return nil, connect.NewError(connect.CodeInternal, errors.New("binary file not initialized"))
			}
			n, err := binaryFile.Write(data.Chunk)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write chunk: %v", err))
			}
			hasher.Write(data.Chunk)
			size += int64(n)
		}
	}

	if metadata == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("no metadata received"))
	}

	// Store binary metadata in database
	metadataJSON, err := json.Marshal(metadata.Metadata)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to marshal metadata: %v", err))
	}

	sha256sum := hex.EncodeToString(hasher.Sum(nil))
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO binary (id, name, version, platform, architecture, size, sha256, metadata, storage_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		binaryID, metadata.Name, metadata.Version, metadata.Platform, metadata.Architecture,
		size, sha256sum, metadataJSON, binaryPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to store binary metadata: %v", err))
	}

	return connect.NewResponse(&pb.UploadBinaryResponse{
		Id:     binaryID,
		Sha256: sha256sum,
	}), nil
}

func (s *BinaryService) GetBinary(ctx context.Context, req *connect.Request[pb.GetBinaryRequest]) (*connect.Response[pb.GetBinaryResponse], error) {
	var (
		binary       pb.Binary
		metadata     string
		createdAtStr sql.NullString
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, version, platform, architecture, size, sha256, metadata,
		 strftime('%Y-%m-%dT%H:%M:%SZ', created_at) as created_at
		 FROM binary WHERE id = ?`,
		req.Msg.Id).Scan(
		&binary.Id, &binary.Name, &binary.Version, &binary.Platform,
		&binary.Architecture, &binary.Size, &binary.Sha256, &metadata, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("binary not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get binary: %v", err))
	}

	if err := json.Unmarshal([]byte(metadata), &binary.Metadata); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal metadata: %v", err))
	}

	if createdAtStr.Valid {
		createdAt, err := time.Parse(time.RFC3339, createdAtStr.String)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse created_at timestamp: %v", err))
		}
		binary.CreatedAt = timestamppb.New(createdAt)
	}

	return connect.NewResponse(&pb.GetBinaryResponse{Binary: &binary}), nil
}

func (s *BinaryService) DownloadBinary(ctx context.Context, req *connect.Request[pb.DownloadBinaryRequest], stream *connect.ServerStream[pb.DownloadBinaryResponse]) error {
	var storagePath string
	err := s.db.QueryRowContext(ctx,
		"SELECT storage_path FROM binary WHERE id = ?", req.Msg.Id).Scan(&storagePath)
	if err == sql.ErrNoRows {
		return connect.NewError(connect.CodeNotFound, errors.New("binary not found"))
	}
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get binary path: %v", err))
	}

	file, err := os.Open(storagePath)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to open binary: %v", err))
	}
	defer file.Close()

	buffer := make([]byte, 32*1024) // 32KB chunks
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read binary: %v", err))
		}

		if err := stream.Send(&pb.DownloadBinaryResponse{
			Chunk: buffer[:n],
		}); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send chunk: %v", err))
		}
	}

	return nil
}

func (s *BinaryService) ListBinaries(ctx context.Context, req *connect.Request[pb.ListBinariesRequest]) (*connect.Response[pb.ListBinariesResponse], error) {
	query := `SELECT id, name, version, platform, architecture, size, sha256, metadata,
			  strftime('%Y-%m-%dT%H:%M:%SZ', created_at) as created_at
			  FROM binary WHERE 1=1`
	args := []any{}

	if req.Msg.Name != "" {
		query += " AND name = ?"
		args = append(args, req.Msg.Name)
	}
	if req.Msg.Version != "" {
		query += " AND version = ?"
		args = append(args, req.Msg.Version)
	}
	if req.Msg.Platform != "" {
		query += " AND platform = ?"
		args = append(args, req.Msg.Platform)
	}
	if req.Msg.Architecture != "" {
		query += " AND architecture = ?"
		args = append(args, req.Msg.Architecture)
	}

	// Add pagination
	if req.Msg.PageSize > 0 {
		query += " LIMIT ?"
		args = append(args, req.Msg.PageSize+1) // Get one extra to determine if there are more pages
	}
	if req.Msg.PageToken != "" {
		query += " AND id > ?"
		args = append(args, req.Msg.PageToken)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list binaries: %v", err))
	}
	defer rows.Close()

	var binaries []*pb.Binary
	for rows.Next() {
		var (
			binary       pb.Binary
			metadata     string
			createdAtStr sql.NullString
		)

		err := rows.Scan(
			&binary.Id, &binary.Name, &binary.Version, &binary.Platform,
			&binary.Architecture, &binary.Size, &binary.Sha256, &metadata, &createdAtStr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scan binary: %v", err))
		}

		if err := json.Unmarshal([]byte(metadata), &binary.Metadata); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to unmarshal metadata: %v", err))
		}

		if createdAtStr.Valid {
			createdAt, err := time.Parse(time.RFC3339, createdAtStr.String)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse created_at timestamp: %v", err))
			}
			binary.CreatedAt = timestamppb.New(createdAt)
		}

		binaries = append(binaries, &binary)
	}

	var nextPageToken string
	if req.Msg.PageSize > 0 && len(binaries) > int(req.Msg.PageSize) {
		nextPageToken = binaries[len(binaries)-1].Id
		binaries = binaries[:len(binaries)-1]
	}

	return connect.NewResponse(&pb.ListBinariesResponse{
		Binaries:      binaries,
		NextPageToken: nextPageToken,
	}), nil
}
