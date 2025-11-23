// Package shutdown provides graceful shutdown handling for the ClickHouse connector.
// It manages connection draining, in-flight request tracking, and cleanup operations.
package shutdown

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ShutdownPhase represents different phases of the shutdown process
type ShutdownPhase int

const (
	PhaseRunning ShutdownPhase = iota
	PhasePreShutdown
	PhaseDraining
	PhaseClosingConnections
	PhaseCleanup
	PhaseTerminated
)

func (p ShutdownPhase) String() string {
	switch p {
	case PhaseRunning:
		return "running"
	case PhasePreShutdown:
		return "pre-shutdown"
	case PhaseDraining:
		return "draining"
	case PhaseClosingConnections:
		return "closing-connections"
	case PhaseCleanup:
		return "cleanup"
	case PhaseTerminated:
		return "terminated"
	default:
		return "unknown"
	}
}

// Config holds configuration for the shutdown manager
type Config struct {
	// GracePeriod is the maximum time to wait for graceful shutdown
	GracePeriod time.Duration

	// DrainTimeout is the time to wait for in-flight requests to complete
	DrainTimeout time.Duration

	// PreShutdownDelay is a delay before starting shutdown (for load balancer deregistration)
	PreShutdownDelay time.Duration

	// ForceTimeout is the absolute maximum time before forcing shutdown
	ForceTimeout time.Duration

	// EnableHealthDegradation marks health check as unhealthy during shutdown
	EnableHealthDegradation bool
}

// DefaultConfig returns the default shutdown configuration
func DefaultConfig() *Config {
	return &Config{
		GracePeriod:             30 * time.Second,
		DrainTimeout:            15 * time.Second,
		PreShutdownDelay:        5 * time.Second,
		ForceTimeout:            60 * time.Second,
		EnableHealthDegradation: true,
	}
}

// Hook represents a shutdown hook function
type Hook struct {
	Name     string
	Priority int // Lower priority runs first
	Fn       func(ctx context.Context) error
}

// Manager handles graceful shutdown of the application
type Manager struct {
	config      *Config
	phase       atomic.Int32
	inFlight    atomic.Int64
	mu          sync.RWMutex
	hooks       []Hook
	closeables  []Closeable
	done        chan struct{}
	shutdownErr error
	logger      Logger
	startTime   time.Time

	// Metrics
	stats ShutdownStats
}

// ShutdownStats holds statistics about the shutdown process
type ShutdownStats struct {
	StartTime            time.Time
	EndTime              time.Time
	TotalDuration        time.Duration
	InFlightAtStart      int64
	InFlightAtEnd        int64
	DrainedRequests      int64
	HooksExecuted        int
	HooksFailed          int
	ConnectionsClosed    int
	ConnectionsTimedOut  int
}

// Closeable is an interface for resources that can be closed
type Closeable interface {
	Close() error
}

// Logger interface for shutdown logging
type Logger interface {
	Printf(format string, v ...interface{})
}

// defaultLogger uses standard log package
type defaultLogger struct{}

func (l *defaultLogger) Printf(format string, v ...interface{}) {
	log.Printf("[SHUTDOWN] "+format, v...)
}

// NewManager creates a new shutdown manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config:     config,
		hooks:      make([]Hook, 0),
		closeables: make([]Closeable, 0),
		done:       make(chan struct{}),
		logger:     &defaultLogger{},
		startTime:  time.Now(),
	}
	m.phase.Store(int32(PhaseRunning))

	return m
}

// SetLogger sets a custom logger
func (m *Manager) SetLogger(logger Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// RegisterHook adds a shutdown hook
func (m *Manager) RegisterHook(hook Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// RegisterCloseable adds a closeable resource
func (m *Manager) RegisterCloseable(name string, c Closeable) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeables = append(m.closeables, c)
}

// TrackRequest marks a request as in-flight
func (m *Manager) TrackRequest() (release func()) {
	m.inFlight.Add(1)
	return func() {
		m.inFlight.Add(-1)
	}
}

// InFlightCount returns the current number of in-flight requests
func (m *Manager) InFlightCount() int64 {
	return m.inFlight.Load()
}

// Phase returns the current shutdown phase
func (m *Manager) Phase() ShutdownPhase {
	return ShutdownPhase(m.phase.Load())
}

// IsShuttingDown returns true if shutdown has been initiated
func (m *Manager) IsShuttingDown() bool {
	return m.Phase() != PhaseRunning
}

// IsHealthy returns false if shutdown is in progress and health degradation is enabled
func (m *Manager) IsHealthy() bool {
	if !m.config.EnableHealthDegradation {
		return true
	}
	return !m.IsShuttingDown()
}

// WaitForShutdown blocks until shutdown is complete
func (m *Manager) WaitForShutdown() error {
	<-m.done
	return m.shutdownErr
}

// Done returns a channel that is closed when shutdown is complete
func (m *Manager) Done() <-chan struct{} {
	return m.done
}

// ListenForSignals starts listening for OS signals and initiates shutdown
func (m *Manager) ListenForSignals(ctx context.Context) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		select {
		case sig := <-signals:
			m.logger.Printf("Received signal: %v", sig)
			m.Shutdown(ctx)
		case <-ctx.Done():
			m.Shutdown(ctx)
		}
	}()
}

// Shutdown initiates the graceful shutdown process
func (m *Manager) Shutdown(ctx context.Context) {
	// Only allow one shutdown
	if !m.phase.CompareAndSwap(int32(PhaseRunning), int32(PhasePreShutdown)) {
		return
	}

	m.stats.StartTime = time.Now()
	m.stats.InFlightAtStart = m.inFlight.Load()

	// Create a context with force timeout
	forceCtx, forceCancel := context.WithTimeout(context.Background(), m.config.ForceTimeout)
	defer forceCancel()

	go m.executeShutdown(forceCtx)
}

// executeShutdown runs the shutdown sequence
func (m *Manager) executeShutdown(ctx context.Context) {
	defer close(m.done)

	var errs []error

	// Phase 1: Pre-shutdown delay (allow load balancers to deregister)
	m.logger.Printf("Phase 1: Pre-shutdown delay (%v)", m.config.PreShutdownDelay)
	if m.config.PreShutdownDelay > 0 {
		select {
		case <-time.After(m.config.PreShutdownDelay):
		case <-ctx.Done():
			m.shutdownErr = ctx.Err()
			return
		}
	}

	// Phase 2: Drain in-flight requests
	m.phase.Store(int32(PhaseDraining))
	m.logger.Printf("Phase 2: Draining in-flight requests (timeout: %v)", m.config.DrainTimeout)

	drainCtx, drainCancel := context.WithTimeout(ctx, m.config.DrainTimeout)
	drainedCount := m.drainRequests(drainCtx)
	drainCancel()
	m.stats.DrainedRequests = drainedCount

	remaining := m.inFlight.Load()
	if remaining > 0 {
		m.logger.Printf("Warning: %d requests still in-flight after drain timeout", remaining)
	}

	// Phase 3: Execute shutdown hooks
	m.phase.Store(int32(PhaseClosingConnections))
	m.logger.Printf("Phase 3: Executing shutdown hooks")

	hookCtx, hookCancel := context.WithTimeout(ctx, m.config.GracePeriod)
	hookErrs := m.executeHooks(hookCtx)
	hookCancel()
	errs = append(errs, hookErrs...)

	// Phase 4: Close connections
	m.logger.Printf("Phase 4: Closing connections")

	closeCtx, closeCancel := context.WithTimeout(ctx, 10*time.Second)
	closeErrs := m.closeConnections(closeCtx)
	closeCancel()
	errs = append(errs, closeErrs...)

	// Phase 5: Cleanup
	m.phase.Store(int32(PhaseCleanup))
	m.logger.Printf("Phase 5: Cleanup")

	// Final stats
	m.stats.EndTime = time.Now()
	m.stats.TotalDuration = m.stats.EndTime.Sub(m.stats.StartTime)
	m.stats.InFlightAtEnd = m.inFlight.Load()

	m.logger.Printf("Shutdown complete in %v", m.stats.TotalDuration)
	m.logger.Printf("Stats: %d requests drained, %d hooks executed (%d failed), %d connections closed",
		m.stats.DrainedRequests, m.stats.HooksExecuted, m.stats.HooksFailed, m.stats.ConnectionsClosed)

	m.phase.Store(int32(PhaseTerminated))

	if len(errs) > 0 {
		m.shutdownErr = fmt.Errorf("shutdown completed with %d errors", len(errs))
	}
}

// drainRequests waits for in-flight requests to complete
func (m *Manager) drainRequests(ctx context.Context) int64 {
	startCount := m.inFlight.Load()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return startCount - m.inFlight.Load()
		case <-ticker.C:
			if m.inFlight.Load() == 0 {
				return startCount
			}
			m.logger.Printf("Waiting for %d in-flight requests...", m.inFlight.Load())
		}
	}
}

// executeHooks runs all registered shutdown hooks
func (m *Manager) executeHooks(ctx context.Context) []error {
	m.mu.RLock()
	hooks := make([]Hook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.RUnlock()

	// Sort by priority (lower first)
	for i := 0; i < len(hooks)-1; i++ {
		for j := i + 1; j < len(hooks); j++ {
			if hooks[j].Priority < hooks[i].Priority {
				hooks[i], hooks[j] = hooks[j], hooks[i]
			}
		}
	}

	var errs []error
	for _, hook := range hooks {
		m.stats.HooksExecuted++
		m.logger.Printf("Executing hook: %s", hook.Name)

		if err := hook.Fn(ctx); err != nil {
			m.stats.HooksFailed++
			m.logger.Printf("Hook %s failed: %v", hook.Name, err)
			errs = append(errs, fmt.Errorf("hook %s: %w", hook.Name, err))
		}
	}

	return errs
}

// closeConnections closes all registered closeables
func (m *Manager) closeConnections(ctx context.Context) []error {
	m.mu.RLock()
	closeables := make([]Closeable, len(m.closeables))
	copy(closeables, m.closeables)
	m.mu.RUnlock()

	var errs []error
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, c := range closeables {
		wg.Add(1)
		go func(c Closeable) {
			defer wg.Done()

			done := make(chan error, 1)
			go func() {
				done <- c.Close()
			}()

			select {
			case err := <-done:
				mu.Lock()
				m.stats.ConnectionsClosed++
				if err != nil {
					errs = append(errs, err)
				}
				mu.Unlock()
			case <-ctx.Done():
				mu.Lock()
				m.stats.ConnectionsTimedOut++
				errs = append(errs, fmt.Errorf("close timed out: %w", ctx.Err()))
				mu.Unlock()
			}
		}(c)
	}

	wg.Wait()
	return errs
}

// GetStats returns shutdown statistics
func (m *Manager) GetStats() ShutdownStats {
	return m.stats
}

// Global default manager
var defaultManager *Manager
var defaultManagerOnce sync.Once

// Default returns the default shutdown manager
func Default() *Manager {
	defaultManagerOnce.Do(func() {
		defaultManager = NewManager(nil)
	})
	return defaultManager
}

// SetDefault sets the default shutdown manager
func SetDefault(m *Manager) {
	defaultManager = m
}
