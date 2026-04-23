package logbuffer

import (
	"strings"
	"sync"
)

type Buffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func New(max int) *Buffer {
	return &Buffer{max: max}
}

// WriteLine appends a single log line.
func (b *Buffer) WriteLine(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
}

// Write implements io.Writer so zapcore.AddSync can wrap Buffer directly.
func (b *Buffer) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	if line != "" {
		b.WriteLine(line)
	}
	return len(p), nil
}

func (b *Buffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

func (b *Buffer) String() string {
	return strings.Join(b.Lines(), "\n")
}
