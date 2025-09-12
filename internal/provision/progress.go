package provision

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
)

// ProgressBarReporter implements ProgressReporter using a proper progress bar
type ProgressBarReporter struct {
	bar     *progressbar.ProgressBar
	verbose bool
}

// NewProgressBarReporter creates a new progress bar reporter
func NewProgressBarReporter(verbose bool) *ProgressBarReporter {
	return &ProgressBarReporter{
		verbose: verbose,
	}
}

// UpdateStatus shows a status message
func (p *ProgressBarReporter) UpdateStatus(status string) {
	// Clear any existing bar
	if p.bar != nil {
		p.bar.Finish()
		p.bar = nil
	}
	fmt.Printf("\n%s\n", status)
}

// UpdateProgress updates the progress bar
func (p *ProgressBarReporter) UpdateProgress(message string, current, total int64) {
	if p.bar == nil || p.bar.GetMax64() != total {
		// Create new bar with proper options
		p.bar = progressbar.NewOptions64(
			total,
			progressbar.OptionSetDescription(message),
			progressbar.OptionSetWriter(os.Stdout),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetWidth(40),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() {
				fmt.Fprint(os.Stdout, "\n")
			}),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
		)
	}

	p.bar.Set64(current)
}

// progressReaderWithBar wraps an io.Reader with progress bar updates
type progressReaderWithBar struct {
	reader io.Reader
	bar    *progressbar.ProgressBar
	read   int64
}

func newProgressReaderWithBar(reader io.Reader, total int64, description string) io.Reader {
	bar := progressbar.NewOptions64(
		total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stdout, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	return &progressReaderWithBar{
		reader: reader,
		bar:    bar,
	}
}

func (r *progressReaderWithBar) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	r.bar.Add(n)
	return n, err
}

// progressWriterWithBar wraps an io.Writer with progress bar updates
type progressWriterWithBar struct {
	writer  io.Writer
	bar     *progressbar.ProgressBar
	written int64
}

func newProgressWriterWithBar(writer io.Writer, total int64, description string) io.Writer {
	bar := progressbar.NewOptions64(
		total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stdout, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	return &progressWriterWithBar{
		writer: writer,
		bar:    bar,
	}
}

func (w *progressWriterWithBar) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.written += int64(n)
	w.bar.Add(n)
	return n, err
}
