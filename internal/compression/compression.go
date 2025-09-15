package compression

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// Compressor handles data compression
type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
	Type() string
}

// GzipCompressor implements gzip compression
type GzipCompressor struct {
	level int
}

// NewGzipCompressor creates a new gzip compressor
func NewGzipCompressor(level int) *GzipCompressor {
	if level < gzip.DefaultCompression || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}
	return &GzipCompressor{level: level}
}

// Compress compresses data using gzip
func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, c.level)
	if err != nil {
		return nil, err
	}

	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Decompress decompresses gzip data
func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// Type returns the compression type
func (c *GzipCompressor) Type() string {
	return "gzip"
}

// ZstdCompressor implements zstd compression
type ZstdCompressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

// NewZstdCompressor creates a new zstd compressor
func NewZstdCompressor(level int) (*ZstdCompressor, error) {
	// Convert level to zstd.EncoderLevel
	var zlevel zstd.EncoderLevel
	switch {
	case level <= 1:
		zlevel = zstd.SpeedFastest
	case level <= 3:
		zlevel = zstd.SpeedDefault
	case level <= 5:
		zlevel = zstd.SpeedBetterCompression
	default:
		zlevel = zstd.SpeedBestCompression
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zlevel))
	if err != nil {
		return nil, err
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		encoder.Close()
		return nil, err
	}

	return &ZstdCompressor{
		encoder: encoder,
		decoder: decoder,
	}, nil
}

// Compress compresses data using zstd
func (c *ZstdCompressor) Compress(data []byte) ([]byte, error) {
	return c.encoder.EncodeAll(data, nil), nil
}

// Decompress decompresses zstd data
func (c *ZstdCompressor) Decompress(data []byte) ([]byte, error) {
	return c.decoder.DecodeAll(data, nil)
}

// Type returns the compression type
func (c *ZstdCompressor) Type() string {
	return "zstd"
}

// Close closes the compressor resources
func (c *ZstdCompressor) Close() error {
	if c.encoder != nil {
		c.encoder.Close()
	}
	if c.decoder != nil {
		c.decoder.Close()
	}
	return nil
}

// NoopCompressor doesn't compress data
type NoopCompressor struct{}

// Compress returns data unchanged
func (c *NoopCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

// Decompress returns data unchanged
func (c *NoopCompressor) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

// Type returns the compression type
func (c *NoopCompressor) Type() string {
	return "none"
}

// New creates a compressor based on type
func New(compressionType string) (Compressor, error) {
	switch compressionType {
	case "gzip":
		return NewGzipCompressor(gzip.DefaultCompression), nil
	case "zstd":
		return NewZstdCompressor(3)
	case "none", "":
		return &NoopCompressor{}, nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compressionType)
	}
}

// NewWithLevel creates a compressor with specific compression level
func NewWithLevel(compressionType string, level int) (Compressor, error) {
	switch compressionType {
	case "gzip":
		return NewGzipCompressor(level), nil
	case "zstd":
		return NewZstdCompressor(level)
	case "none", "":
		return &NoopCompressor{}, nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compressionType)
	}
}