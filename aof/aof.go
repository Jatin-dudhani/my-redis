package aof

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Jatin-dudhani/my-redis/resp"
)

type FsyncPolicy string

const (
	FsyncAlways   FsyncPolicy = "always"
	FsyncEverySec FsyncPolicy = "everysec"
	FsyncNo       FsyncPolicy = "no"
)

type AOF struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	writer *bufio.Writer
	policy FsyncPolicy
	stopCh chan struct{}
}

func Open(path string, policy FsyncPolicy) (*AOF, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open aof: %w", err)
	}

	aof := &AOF{
		path:   path,
		file:   f,
		writer: bufio.NewWriterSize(f, 65536),
		policy: policy,
		stopCh: make(chan struct{}),
	}

	if policy == FsyncEverySec {
		go aof.backgroundFlusher()
	}

	return aof, nil
}

func (a *AOF) backgroundFlusher() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.mu.Lock()
			if a.file != nil {
				a.writer.Flush()
				_ = a.file.Sync()
			}
			a.mu.Unlock()
		case <-a.stopCh:
			return
		}
	}
}

func (a *AOF) Write(v resp.Value) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file == nil {
		return fmt.Errorf("aof is closed")
	}

	var buf bytes.Buffer
	w := resp.NewWriter(&buf)
	if err := w.Write(v); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}

	if _, err := a.writer.Write(buf.Bytes()); err != nil {
		return err
	}

	if a.policy == FsyncAlways {
		if err := a.writer.Flush(); err != nil {
			return err
		}
		return a.file.Sync()
	}

	return nil
}

func (a *AOF) Flush() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file == nil {
		return nil
	}
	if err := a.writer.Flush(); err != nil {
		return err
	}
	return a.file.Sync()
}

func (a *AOF) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopCh != nil {
		select {
		case <-a.stopCh:
		default:
			close(a.stopCh)
		}
	}

	if a.file == nil {
		return nil
	}

	_ = a.writer.Flush()
	_ = a.file.Sync()
	err := a.file.Close()
	a.file = nil
	return err
}

func (a *AOF) Replay(fn func(v resp.Value) error) error {
	a.mu.Lock()
	_ = a.writer.Flush()
	a.mu.Unlock()

	f, err := os.Open(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	reader := resp.NewReader(f)
	for {
		v, err := reader.Read()
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return err
		}
		if err := fn(v); err != nil {
			return err
		}
	}
	return nil
}

// Rewrite rewrites the AOF file with the given minimal commands atomically.
func (a *AOF) Rewrite(commands []resp.Value) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	tmpPath := a.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create tmp aof: %w", err)
	}

	tmpWriter := bufio.NewWriterSize(tmpFile, 65536)
	w := resp.NewWriter(tmpWriter)
	for _, cmd := range commands {
		if err := w.Write(cmd); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return err
		}
	}
	if err := w.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	if err := tmpWriter.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	tmpFile.Close()

	// Close existing AOF file
	_ = a.writer.Flush()
	_ = a.file.Sync()
	_ = a.file.Close()

	// Atomically swap files
	if err := os.Rename(tmpPath, a.path); err != nil {
		return fmt.Errorf("rename tmp aof: %w", err)
	}

	// Reopen swapped file
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("reopen swapped aof: %w", err)
	}
	a.file = f
	a.writer = bufio.NewWriterSize(f, 65536)

	return nil
}
