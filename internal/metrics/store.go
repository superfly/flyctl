package metrics

import (
	"bufio"
	"context"
	"encoding/json"
	"sort"
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
	Payload   json.RawMessage `json:"p,omitempty"`
	Timestamp time.Time       `json:"t,omitempty"`
}

type Cache interface {
	// Append appends the given entries to the store'c buffer.
	Write(entry *Entry) (int, error)
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

	file, err := createMetricsFile()
	if err != nil {
		return nil, err
	}

	c.filePath = file.Name()

	c.current = file

	c.buffer = bufio.NewWriter(file)

	return c, nil
}

func (c *cache) Write(entry *Entry) (int, error) {
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

	var entries = make([]Entry, 0)
	var filesRead = make([]string, 0)

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && path != dir {
			return filepath.SkipDir
		}

		if strings.HasPrefix(info.Name(), "flyctl-metrics") && !strings.HasSuffix(info.Name(), ".tmp") {
			file, err := os.Open(path)
			if err != nil {
				return nil
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

			filesRead = append(filesRead, path)
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, filesRead, nil
}

// Purge deletes specific metrics files from the list.
func Purge(files []string) error {
	dir, err := metricsDir()
	if err != nil {
		return err
	}

	unlock, err := filemu.RLock(context.Background(), filepath.Join(dir, lockFileName))
	if err != nil {
		return err
	}
	defer unlock()

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	return nil
}
