package sync

import (
	"fmt"

	pb "fleetd.sh/gen/fleetd/v1"
	"fleetd.sh/internal/compression"
	"google.golang.org/protobuf/proto"
)

// compressBatch compresses a metrics batch
func (m *Manager) compressBatch(batch *pb.MetricsBatch) (*pb.MetricsBatch, error) {
	// Marshal the metrics to bytes
	data, err := proto.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch: %w", err)
	}

	originalSize := len(data)

	// Get compressor
	compressor, err := compression.New(m.config.CompressionType)
	if err != nil {
		return nil, err
	}

	// For zstd, close resources when done
	if zc, ok := compressor.(*compression.ZstdCompressor); ok {
		defer zc.Close()
	}

	// Compress data
	compressed, err := compressor.Compress(data)
	if err != nil {
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	// Check if compression is worth it (at least 10% reduction)
	if float64(len(compressed))/float64(originalSize) > 0.9 {
		// Not worth compressing
		return batch, nil
	}

	// Create compressed batch
	return &pb.MetricsBatch{
		Compression:    compressor.Type(),
		CompressedData: compressed,
		OriginalSize:   int32(originalSize),
	}, nil
}

// decompressBatch decompresses a metrics batch
func decompressBatch(batch *pb.MetricsBatch) (*pb.MetricsBatch, error) {
	if batch.Compression == "" || batch.Compression == "none" {
		return batch, nil
	}

	// Get compressor
	compressor, err := compression.New(batch.Compression)
	if err != nil {
		return nil, err
	}

	// For zstd, close resources when done
	if zc, ok := compressor.(*compression.ZstdCompressor); ok {
		defer zc.Close()
	}

	// Decompress data
	data, err := compressor.Decompress(batch.CompressedData)
	if err != nil {
		return nil, fmt.Errorf("decompression failed: %w", err)
	}

	// Unmarshal the metrics
	var decompressed pb.MetricsBatch
	if err := proto.Unmarshal(data, &decompressed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal batch: %w", err)
	}

	return &decompressed, nil
}
