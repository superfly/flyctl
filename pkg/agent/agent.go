package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func IsIPv6(addr string) bool {
	addr = strings.Trim(addr, "[]")
	ip := net.ParseIP(addr)
	return ip != nil && ip.To16() != nil
}

func pidFile() string {
	return filepath.Join(userHome(), ".fly", "agent.pid")
}

func userHome() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return dir
}

func getRunningPid() (int, error) {
	data, err := os.ReadFile(pidFile())
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

func setRunningPid(pid int) error {
	return os.WriteFile(pidFile(), []byte(strconv.Itoa(pid)), 0666)
}

func CreatePidFile() error {
	return setRunningPid(os.Getpid())
}

func RemovePidFile(logger *log.Logger) (err error) {
	var pid int
	if pid, _ = getRunningPid(); pid != os.Getpid() {
		return
	}

	if err = os.Remove(pidFile()); err != nil {
		err = fmt.Errorf("failed removing pid file: %w", err)

		logger.Print(err)
	}

	return
}

func StopRunningAgent() (err error) {
	var process *os.Process
	if process, err = runningProcess(); err != nil || process == nil {
		return
	}

	if err = process.Signal(os.Interrupt); errors.Is(err, os.ErrProcessDone) {
		err = nil
	}

	return
}

func runningProcess() (*os.Process, error) {
	pid, err := getRunningPid()
	if err != nil {
		return nil, err
	}
	if pid == 0 {
		return nil, nil
	}

	return os.FindProcess(pid)
}

func PathToSocket() string {
	return filepath.Join(userHome(), ".fly", "fly-agent.sock")
}

func PathToLock() string {
	return filepath.Join(os.TempDir(), "fly-agent.lock")
}

type Instances struct {
	Labels    []string
	Addresses []string
}
