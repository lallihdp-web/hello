package shutdown

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager(nil)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.Phase() != PhaseRunning {
		t.Errorf("expected phase running, got %v", m.Phase())
	}
}

func TestManagerWithConfig(t *testing.T) {
	cfg := &Config{
		GracePeriod:      10 * time.Second,
		DrainTimeout:     5 * time.Second,
		PreShutdownDelay: 1 * time.Second,
		ForceTimeout:     30 * time.Second,
	}
	m := NewManager(cfg)
	if m.config.GracePeriod != 10*time.Second {
		t.Errorf("expected grace period 10s, got %v", m.config.GracePeriod)
	}
}

func TestTrackRequest(t *testing.T) {
	m := NewManager(nil)

	if m.InFlightCount() != 0 {
		t.Errorf("expected 0 in-flight, got %d", m.InFlightCount())
	}

	release1 := m.TrackRequest()
	release2 := m.TrackRequest()

	if m.InFlightCount() != 2 {
		t.Errorf("expected 2 in-flight, got %d", m.InFlightCount())
	}

	release1()
	if m.InFlightCount() != 1 {
		t.Errorf("expected 1 in-flight, got %d", m.InFlightCount())
	}

	release2()
	if m.InFlightCount() != 0 {
		t.Errorf("expected 0 in-flight, got %d", m.InFlightCount())
	}
}

func TestIsShuttingDown(t *testing.T) {
	m := NewManager(&Config{
		GracePeriod:      100 * time.Millisecond,
		DrainTimeout:     50 * time.Millisecond,
		PreShutdownDelay: 0,
		ForceTimeout:     500 * time.Millisecond,
	})

	if m.IsShuttingDown() {
		t.Error("expected not shutting down initially")
	}

	m.Shutdown(context.Background())
	time.Sleep(10 * time.Millisecond)

	if !m.IsShuttingDown() {
		t.Error("expected shutting down after Shutdown called")
	}

	<-m.Done()
}

func TestShutdownDrainsRequests(t *testing.T) {
	m := NewManager(&Config{
		GracePeriod:      100 * time.Millisecond,
		DrainTimeout:     500 * time.Millisecond,
		PreShutdownDelay: 0,
		ForceTimeout:     1 * time.Second,
	})

	// Start a simulated request
	var released atomic.Bool
	release := m.TrackRequest()
	go func() {
		time.Sleep(100 * time.Millisecond)
		release()
		released.Store(true)
	}()

	// Start shutdown
	m.Shutdown(context.Background())

	// Wait for shutdown to complete
	<-m.Done()

	if !released.Load() {
		t.Error("expected request to be released")
	}
	if m.InFlightCount() != 0 {
		t.Errorf("expected 0 in-flight after drain, got %d", m.InFlightCount())
	}
}

func TestShutdownHooks(t *testing.T) {
	m := NewManager(&Config{
		GracePeriod:      500 * time.Millisecond,
		DrainTimeout:     50 * time.Millisecond,
		PreShutdownDelay: 0,
		ForceTimeout:     1 * time.Second,
	})

	var hookCalled atomic.Bool
	m.RegisterHook(Hook{
		Name:     "test-hook",
		Priority: 1,
		Fn: func(ctx context.Context) error {
			hookCalled.Store(true)
			return nil
		},
	})

	m.Shutdown(context.Background())
	<-m.Done()

	if !hookCalled.Load() {
		t.Error("expected hook to be called")
	}
}

func TestShutdownHookPriority(t *testing.T) {
	m := NewManager(&Config{
		GracePeriod:      500 * time.Millisecond,
		DrainTimeout:     50 * time.Millisecond,
		PreShutdownDelay: 0,
		ForceTimeout:     1 * time.Second,
	})

	var order []int
	var mu sync.Mutex

	m.RegisterHook(Hook{
		Name:     "hook-3",
		Priority: 3,
		Fn: func(ctx context.Context) error {
			mu.Lock()
			order = append(order, 3)
			mu.Unlock()
			return nil
		},
	})
	m.RegisterHook(Hook{
		Name:     "hook-1",
		Priority: 1,
		Fn: func(ctx context.Context) error {
			mu.Lock()
			order = append(order, 1)
			mu.Unlock()
			return nil
		},
	})
	m.RegisterHook(Hook{
		Name:     "hook-2",
		Priority: 2,
		Fn: func(ctx context.Context) error {
			mu.Lock()
			order = append(order, 2)
			mu.Unlock()
			return nil
		},
	})

	m.Shutdown(context.Background())
	<-m.Done()

	if len(order) != 3 {
		t.Fatalf("expected 3 hooks, got %d", len(order))
	}
	if order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected order [1,2,3], got %v", order)
	}
}

func TestShutdownHookError(t *testing.T) {
	m := NewManager(&Config{
		GracePeriod:      500 * time.Millisecond,
		DrainTimeout:     50 * time.Millisecond,
		PreShutdownDelay: 0,
		ForceTimeout:     1 * time.Second,
	})

	m.RegisterHook(Hook{
		Name:     "failing-hook",
		Priority: 1,
		Fn: func(ctx context.Context) error {
			return errors.New("hook failed")
		},
	})

	m.Shutdown(context.Background())
	err := m.WaitForShutdown()

	if err == nil {
		t.Error("expected error from failed hook")
	}
}

type mockCloseable struct {
	closed atomic.Bool
	delay  time.Duration
}

func (m *mockCloseable) Close() error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.closed.Store(true)
	return nil
}

func TestShutdownCloseables(t *testing.T) {
	m := NewManager(&Config{
		GracePeriod:      500 * time.Millisecond,
		DrainTimeout:     50 * time.Millisecond,
		PreShutdownDelay: 0,
		ForceTimeout:     1 * time.Second,
	})

	c1 := &mockCloseable{}
	c2 := &mockCloseable{}

	m.RegisterCloseable("conn1", c1)
	m.RegisterCloseable("conn2", c2)

	m.Shutdown(context.Background())
	<-m.Done()

	if !c1.closed.Load() {
		t.Error("expected conn1 to be closed")
	}
	if !c2.closed.Load() {
		t.Error("expected conn2 to be closed")
	}
}

func TestShutdownStats(t *testing.T) {
	m := NewManager(&Config{
		GracePeriod:      100 * time.Millisecond,
		DrainTimeout:     50 * time.Millisecond,
		PreShutdownDelay: 0,
		ForceTimeout:     500 * time.Millisecond,
	})

	m.RegisterHook(Hook{Name: "hook1", Fn: func(ctx context.Context) error { return nil }})
	m.RegisterCloseable("conn1", &mockCloseable{})

	release := m.TrackRequest()
	go func() {
		time.Sleep(10 * time.Millisecond)
		release()
	}()

	m.Shutdown(context.Background())
	<-m.Done()

	stats := m.GetStats()
	if stats.HooksExecuted != 1 {
		t.Errorf("expected 1 hook executed, got %d", stats.HooksExecuted)
	}
	if stats.ConnectionsClosed != 1 {
		t.Errorf("expected 1 connection closed, got %d", stats.ConnectionsClosed)
	}
	if stats.TotalDuration <= 0 {
		t.Error("expected positive total duration")
	}
}

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase    ShutdownPhase
		expected string
	}{
		{PhaseRunning, "running"},
		{PhasePreShutdown, "pre-shutdown"},
		{PhaseDraining, "draining"},
		{PhaseClosingConnections, "closing-connections"},
		{PhaseCleanup, "cleanup"},
		{PhaseTerminated, "terminated"},
		{ShutdownPhase(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.expected {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.expected)
		}
	}
}

func TestIsHealthy(t *testing.T) {
	m := NewManager(&Config{
		EnableHealthDegradation: true,
		PreShutdownDelay:        0,
		DrainTimeout:            50 * time.Millisecond,
		GracePeriod:             100 * time.Millisecond,
		ForceTimeout:            500 * time.Millisecond,
	})

	if !m.IsHealthy() {
		t.Error("expected healthy before shutdown")
	}

	m.Shutdown(context.Background())
	time.Sleep(10 * time.Millisecond)

	if m.IsHealthy() {
		t.Error("expected unhealthy during shutdown")
	}

	<-m.Done()
}

func TestDefaultManager(t *testing.T) {
	m1 := Default()
	m2 := Default()

	if m1 != m2 {
		t.Error("expected same default manager instance")
	}
}

func BenchmarkTrackRequest(b *testing.B) {
	m := NewManager(nil)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		release := m.TrackRequest()
		release()
	}
}

func BenchmarkInFlightCount(b *testing.B) {
	m := NewManager(nil)
	for i := 0; i < 100; i++ {
		m.TrackRequest()
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = m.InFlightCount()
	}
}
