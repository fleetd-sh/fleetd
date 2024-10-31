package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	storagepb "fleetd.sh/gen/storage/v1"
	"fleetd.sh/internal/telemetry"
)

type StorageService struct {
	basePath string
}

func NewStorageService(basePath string) *StorageService {
	return &StorageService{
		basePath: basePath,
	}
}

func (s *StorageService) PutObject(
	ctx context.Context,
	req *connect.Request[storagepb.PutObjectRequest],
) (*connect.Response[storagepb.PutObjectResponse], error) {
	defer telemetry.TrackDiskOperation(ctx, "PutObject")(nil)

	bucket := req.Msg.Bucket
	key := req.Msg.Key
	data := req.Msg.Data

	path := filepath.Join(s.basePath, bucket, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create directory: %w", err))
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write file: %w", err))
	}

	return connect.NewResponse(&storagepb.PutObjectResponse{
		Success: true,
		Message: "Object stored successfully",
	}), nil
}

func (s *StorageService) GetObject(
	ctx context.Context,
	req *connect.Request[storagepb.GetObjectRequest],
) (*connect.Response[storagepb.GetObjectResponse], error) {
	defer telemetry.TrackDiskOperation(ctx, "GetObject")(nil)

	bucket := req.Msg.Bucket
	key := req.Msg.Key

	path := filepath.Join(s.basePath, bucket, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("object not found: %w", err))
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get file info: %w", err))
	}

	return connect.NewResponse(&storagepb.GetObjectResponse{
		Data:         data,
		LastModified: timestamppb.New(fileInfo.ModTime()),
	}), nil
}

func (s *StorageService) ListObjects(
	ctx context.Context,
	req *connect.Request[storagepb.ListObjectsRequest],
	stream *connect.ServerStream[storagepb.ListObjectsResponse],
) error {
	defer telemetry.TrackDiskOperation(ctx, "ListObjects")(nil)

	bucket := req.Msg.Bucket
	prefix := req.Msg.Prefix

	path := filepath.Join(s.basePath, bucket, prefix)

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(filepath.Join(s.basePath, bucket), filePath)
			stream.Send(&storagepb.ListObjectsResponse{
				Object: &storagepb.ObjectInfo{
					Key:          relPath,
					Size:         info.Size(),
					LastModified: timestamppb.New(info.ModTime()),
				},
			})
		}
		return nil
	})

	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list objects: %w", err))
	}

	return nil
}

func (s *StorageService) DeleteObject(
	ctx context.Context,
	req *connect.Request[storagepb.DeleteObjectRequest],
) (*connect.Response[storagepb.DeleteObjectResponse], error) {
	defer telemetry.TrackDiskOperation(ctx, "DeleteObject")(nil)

	bucket := req.Msg.Bucket
	key := req.Msg.Key

	path := filepath.Join(s.basePath, bucket, key)
	err := os.Remove(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete object: %w", err))
	}

	return connect.NewResponse(&storagepb.DeleteObjectResponse{
		Success: true,
		Message: "Object deleted successfully",
	}), nil
}
