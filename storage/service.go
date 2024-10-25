package storage

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	storagepb "fleetd.sh/gen/storage/v1"
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
	bucket := req.Msg.Bucket
	key := req.Msg.Key
	data := req.Msg.Data

	path := filepath.Join(s.basePath, bucket, key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create directory: %v", err))
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write file: %v", err))
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
	bucket := req.Msg.Bucket
	key := req.Msg.Key

	path := filepath.Join(s.basePath, bucket, key)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("object not found: %v", err))
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get file info: %v", err))
	}

	return connect.NewResponse(&storagepb.GetObjectResponse{
		Data:         data,
		LastModified: timestamppb.New(fileInfo.ModTime()),
	}), nil
}

func (s *StorageService) ListObjects(
	ctx context.Context,
	req *connect.Request[storagepb.ListObjectsRequest],
) (*connect.Response[storagepb.ListObjectsResponse], error) {
	bucket := req.Msg.Bucket
	prefix := req.Msg.Prefix

	path := filepath.Join(s.basePath, bucket, prefix)
	var objects []*storagepb.ObjectInfo

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(filepath.Join(s.basePath, bucket), filePath)
			objects = append(objects, &storagepb.ObjectInfo{
				Key:          relPath,
				Size:         info.Size(),
				LastModified: timestamppb.New(info.ModTime()),
			})
		}
		return nil
	})

	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list objects: %v", err))
	}

	return connect.NewResponse(&storagepb.ListObjectsResponse{
		Objects: objects,
	}), nil
}

func (s *StorageService) DeleteObject(
	ctx context.Context,
	req *connect.Request[storagepb.DeleteObjectRequest],
) (*connect.Response[storagepb.DeleteObjectResponse], error) {
	bucket := req.Msg.Bucket
	key := req.Msg.Key

	path := filepath.Join(s.basePath, bucket, key)
	err := os.Remove(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete object: %v", err))
	}

	return connect.NewResponse(&storagepb.DeleteObjectResponse{
		Success: true,
		Message: "Object deleted successfully",
	}), nil
}
