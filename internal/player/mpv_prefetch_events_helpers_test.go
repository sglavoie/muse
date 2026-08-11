package player

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func fakeEventCommandServer(t *testing.T, socketPath string, mu *sync.Mutex, received *[]ipcTestCommand) chan<- string {
	t.Helper()

	events := make(chan string, 8)
	done := make(chan struct{})

	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		close(done)
		ln.Close()
		os.Remove(socketPath)
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()

				dec := json.NewDecoder(conn)
				enc := json.NewEncoder(conn)
				for {
					var req ipcTestRequest
					if err := dec.Decode(&req); err != nil {
						return
					}
					recordCommand(mu, received, req.Command)
					if req.Command.stringIs(0, cmdObserveProperty) {
						_ = enc.Encode(successHandler(req))
						for {
							select {
							case event := <-events:
								if _, err := fmt.Fprintln(conn, event); err != nil {
									return
								}
							case <-done:
								return
							}
						}
					}
					_ = enc.Encode(successHandler(req))
				}
			}(conn)
		}
	}()

	return events
}

func waitForCurrentIndex(t *testing.T, m *Manager, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.mu.RLock()
	got := m.currentIndex
	m.mu.RUnlock()
	t.Fatalf("currentIndex = %d, want %d", got, want)
}

func waitForCommandCount(t *testing.T, mu *sync.Mutex, received *[]ipcTestCommand, match func(ipcTestCommand) bool, want int, timeout time.Duration, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := 0
		for _, cmd := range *received {
			if match(cmd) {
				count++
			}
		}
		mu.Unlock()
		if count >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("timed out waiting for %d %s commands; got %v", want, desc, *received)
}
