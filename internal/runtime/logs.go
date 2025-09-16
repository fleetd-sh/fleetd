package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func newLogManager(name string, baseDir string, maxSize int64, keepFiles int) (*logManager, error) {
	logDir := filepath.Join(baseDir, "logs", name)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	stdout, err := os.OpenFile(
		filepath.Join(logDir, "stdout.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout log: %w", err)
	}

	stderr, err := os.OpenFile(
		filepath.Join(logDir, "stderr.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr log: %w", err)
	}

	return &logManager{
		stdout:     stdout,
		stderr:     stderr,
		maxSize:    maxSize,
		keepFiles:  keepFiles,
		logDir:     logDir,
		currentGen: 0,
	}, nil
}

func (lm *logManager) rotate(isStdout bool) error {
	var file *os.File
	var baseName string
	if isStdout {
		file = lm.stdout
		baseName = "stdout.log"
	} else {
		file = lm.stderr
		baseName = "stderr.log"
	}

	// Close current file
	file.Close()

	// Rotate existing files
	for i := lm.keepFiles - 1; i >= 0; i-- {
		oldPath := filepath.Join(lm.logDir, baseName+"."+strconv.Itoa(i))
		newPath := filepath.Join(lm.logDir, baseName+"."+strconv.Itoa(i+1))

		// Remove oldest file if it exists
		if i == lm.keepFiles-1 {
			os.Remove(newPath)
			continue
		}

		// Rename existing files
		if _, err := os.Stat(oldPath); err == nil {
			os.Rename(oldPath, newPath)
		}
	}

	// Rename current log file
	currentPath := filepath.Join(lm.logDir, baseName)
	newPath := filepath.Join(lm.logDir, baseName+".0")
	if err := os.Rename(currentPath, newPath); err != nil {
		return fmt.Errorf("failed to rotate log file: %w", err)
	}

	// Create new log file
	newFile, err := os.OpenFile(currentPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create new log file: %w", err)
	}

	// Update file pointer
	if isStdout {
		lm.stdout = newFile
	} else {
		lm.stderr = newFile
	}

	return nil
}

func (lm *logManager) checkRotate() error {
	// Check stdout size
	if stat, err := lm.stdout.Stat(); err == nil && stat.Size() > lm.maxSize {
		if err := lm.rotate(true); err != nil {
			return err
		}
	}

	// Check stderr size
	if stat, err := lm.stderr.Stat(); err == nil && stat.Size() > lm.maxSize {
		if err := lm.rotate(false); err != nil {
			return err
		}
	}

	return nil
}
