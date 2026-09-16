package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type traderProcess struct {
	cmd     *exec.Cmd
	started time.Time
	active  atomic.Bool
	exited  chan struct{}
	err     error
}

func startTraderProcess(cfg TraderConfig, logger *log.Logger) (*traderProcess, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("trader command is empty")
	}
	command := cfg.Command
	cmd := exec.Command(command[0], command[1:]...)
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}
	proc := &traderProcess{cmd: cmd, started: time.Now(), exited: make(chan struct{})}
	writer := newMarkerWriter(os.Stdout, cfg.StartupMarker, func() { proc.active.Store(true) })
	cmd.Stdout = writer
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start trader: %w", err)
	}
	go func() {
		proc.err = cmd.Wait()
		close(proc.exited)
	}()
	return proc, nil
}

func (p *traderProcess) alive() bool {
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

func (p *traderProcess) waitExit(timeout time.Duration) bool {
	select {
	case <-p.exited:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (p *traderProcess) signal(sig syscall.Signal) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, sig)
}

func (p *traderProcess) stop(grace time.Duration, logger *log.Logger) {
	if p == nil {
		return
	}
	p.signal(syscall.SIGTERM)
	if p.waitExit(grace) {
		return
	}
	if logger != nil {
		logger.Printf("watchdog: trader pid=%d did not stop within %s, killing", p.cmd.Process.Pid, grace)
	}
	p.signal(syscall.SIGKILL)
	<-p.exited
}

type markerWriter struct {
	out    io.Writer
	marker []byte
	onMark func()

	mu    sync.Mutex
	buf   []byte
	fired bool
}

func newMarkerWriter(out io.Writer, marker string, onMark func()) *markerWriter {
	return &markerWriter{out: out, marker: []byte(marker), onMark: onMark}
}

func (m *markerWriter) Write(p []byte) (int, error) {
	n, err := m.out.Write(p)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf = append(m.buf, p...)
	for {
		index := bytes.IndexByte(m.buf, '\n')
		if index < 0 {
			break
		}
		line := m.buf[:index]
		m.buf = m.buf[index+1:]
		if !m.fired && len(m.marker) > 0 && bytes.Contains(line, m.marker) {
			m.fired = true
			if m.onMark != nil {
				m.onMark()
			}
		}
	}
	if len(m.buf) > 1<<20 {
		m.buf = m.buf[:0]
	}
	return n, err
}
