package metrics

import (
	"bufio"
	"context"
	"encoding/json"
	"sync"

	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superfly/flyctl/internal/filemu"
)

const (
	lockFileName = "metrics.lock"
)

type Entry struct {
	Metric    string          `json:"m"`
	Payload   json.RawMessage `json:"p"`
	Timestamp time.Time       `json:"t,omitempty"`
}

type Cache interface {
	// Append appends the given entries to the store'c buffer.
	Write(entry Entry) (int, error)
	// Flush flushes the buffer to a file.
	Flush() error
}

type cache struct {
	lock     sync.Mutex
	filePath string
	current  *os.File
	buffer   *bufio.Writer
}

func New() (Cache, error) {
	c := &cache{}

	dir, err := metricsDir()
	if err != nil {
		return nil, err
	}

	file, err := createMetricsFile()
	if err != nil {
		return nil, err
	}

	c.filePath = filepath.Join(dir, file.Name())

	c.current = file

	c.buffer = bufio.NewWriter(file)

	return c, nil
}

func (c *cache) Write(entry Entry) (int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	n, err := c.buffer.WriteString(string(data) + "\n")
	return n, err
}

func (c *cache) Flush() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if err := c.buffer.Flush(); err != nil {
		return err
	}

	// Close the file
	c.current.Close()

	// Rename the file to remove the .tmp suffix
	finalPath := strings.TrimSuffix(c.filePath, ".tmp")
	return os.Rename(c.filePath, finalPath)
}

// Load all metric files into a single buffer and return the list of files read.
func Load() ([]Entry, []string, error) {
	dir, err := metricsDir()

	if err != nil {
		return nil, nil, err
	}

	unlock, err := filemu.RLock(context.Background(), filepath.Join(dir, lockFileName))
	if err != nil {
		return nil, nil, err
	}
	defer unlock()

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	var entries = make([]Entry, 0)
	var filesRead = make([]string, 0)

	for _, entry := range dirEntries {
		if !entry.IsDir() && !strings.HasSuffix(entry.Name(), ".tmp") {
			filePath := filepath.Join(dir, entry.Name())

			// Read and decode entries from the file
			file, err := os.Open(filePath)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				var e Entry
				line := scanner.Text()
				if err := json.Unmarshal([]byte(line), &e); err != nil {
					continue
				}
				entries = append(entries, e)
			}
			file.Close()

			// Add the file path to the list of files read
			filesRead = append(filesRead, filePath)
		}
	}
	return entries, filesRead, nil
}

// Purge deletes specific metrics files from the list.
func Purge(files []string) error {
	// Path to the lock file
	lockFilePath := filepath.Join(baseDir, lockFileName)

	// Acquire an exclusive lock on the lock file to ensure no other process is reading/writing
	unlock, err := filemu.Lock(context.Background(), lockFilePath)
	if err != nil {
		return err
	}
	defer unlock()

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			// Handle error or continue based on your preference
			continue
		}
	}

	return nil
}
