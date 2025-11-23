package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
)

// Manager handles GraphQL subscriptions using polling
type Manager struct {
	client        *clickhouse.Client
	subscriptions map[string]*Subscription
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// Subscription represents an active subscription
type Subscription struct {
	ID           string
	Query        string
	Variables    map[string]interface{}
	Interval     time.Duration
	LastResult   interface{}
	LastHash     string
	Callbacks    []func(data interface{})
	CreatedAt    time.Time
	LastPolledAt time.Time
	ErrorCount   int
	Active       bool
	mu           sync.Mutex
}

// SubscriptionConfig holds subscription configuration
type SubscriptionConfig struct {
	// Default polling interval
	DefaultInterval time.Duration

	// Minimum allowed polling interval
	MinInterval time.Duration

	// Maximum allowed polling interval
	MaxInterval time.Duration

	// Maximum subscriptions per connection
	MaxSubscriptions int

	// Enable deduplication (only send when data changes)
	EnableDeduplication bool
}

// DefaultConfig returns the default subscription configuration
func DefaultConfig() *SubscriptionConfig {
	return &SubscriptionConfig{
		DefaultInterval:     time.Second * 5,
		MinInterval:         time.Second * 1,
		MaxInterval:         time.Minute * 5,
		MaxSubscriptions:    100,
		EnableDeduplication: true,
	}
}

// NewManager creates a new subscription manager
func NewManager(client *clickhouse.Client) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		client:        client,
		subscriptions: make(map[string]*Subscription),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start the polling loop
	go m.pollLoop()

	return m
}

// Subscribe creates a new subscription
func (m *Manager) Subscribe(id, query string, variables map[string]interface{}, interval time.Duration, callback func(data interface{})) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if subscription already exists
	if sub, exists := m.subscriptions[id]; exists {
		sub.mu.Lock()
		sub.Callbacks = append(sub.Callbacks, callback)
		sub.mu.Unlock()
		return nil
	}

	// Validate interval
	config := DefaultConfig()
	if interval < config.MinInterval {
		interval = config.MinInterval
	}
	if interval > config.MaxInterval {
		interval = config.MaxInterval
	}

	// Check max subscriptions
	if len(m.subscriptions) >= config.MaxSubscriptions {
		return fmt.Errorf("maximum subscriptions reached (%d)", config.MaxSubscriptions)
	}

	sub := &Subscription{
		ID:         id,
		Query:      query,
		Variables:  variables,
		Interval:   interval,
		Callbacks:  []func(data interface{}){callback},
		CreatedAt:  time.Now(),
		Active:     true,
	}

	m.subscriptions[id] = sub

	// Execute initial query
	go m.pollSubscription(sub)

	return nil
}

// Unsubscribe removes a subscription
func (m *Manager) Unsubscribe(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sub, exists := m.subscriptions[id]; exists {
		sub.mu.Lock()
		sub.Active = false
		sub.mu.Unlock()
		delete(m.subscriptions, id)
	}
}

// GetSubscription returns a subscription by ID
func (m *Manager) GetSubscription(id string) (*Subscription, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sub, exists := m.subscriptions[id]
	return sub, exists
}

// ListSubscriptions returns all active subscriptions
func (m *Manager) ListSubscriptions() []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		result = append(result, sub)
	}
	return result
}

// Close stops all subscriptions and cleans up
func (m *Manager) Close() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sub := range m.subscriptions {
		sub.mu.Lock()
		sub.Active = false
		sub.mu.Unlock()
		delete(m.subscriptions, id)
	}
}

// pollLoop continuously polls all subscriptions
func (m *Manager) pollLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.pollAll()
		}
	}
}

// pollAll polls all subscriptions that are due
func (m *Manager) pollAll() {
	m.mu.RLock()
	subs := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		subs = append(subs, sub)
	}
	m.mu.RUnlock()

	now := time.Now()
	for _, sub := range subs {
		sub.mu.Lock()
		if sub.Active && (sub.LastPolledAt.IsZero() || now.Sub(sub.LastPolledAt) >= sub.Interval) {
			sub.mu.Unlock()
			go m.pollSubscription(sub)
		} else {
			sub.mu.Unlock()
		}
	}
}

// pollSubscription executes a single subscription poll
func (m *Manager) pollSubscription(sub *Subscription) {
	sub.mu.Lock()
	if !sub.Active {
		sub.mu.Unlock()
		return
	}
	sub.LastPolledAt = time.Now()
	sub.mu.Unlock()

	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	// Execute the query
	rows, err := m.client.Query(ctx, sub.Query)
	if err != nil {
		sub.mu.Lock()
		sub.ErrorCount++
		sub.mu.Unlock()
		return
	}
	defer rows.Close()

	// Collect results
	var results []map[string]interface{}
	columnTypes := rows.ColumnTypes()
	columns := make([]string, len(columnTypes))
	for i, ct := range columnTypes {
		columns[i] = ct.Name()
	}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	// Check for changes (deduplication)
	config := DefaultConfig()
	if config.EnableDeduplication {
		newHash := hashResults(results)
		sub.mu.Lock()
		if newHash == sub.LastHash {
			sub.mu.Unlock()
			return // No changes
		}
		sub.LastHash = newHash
		sub.LastResult = results
		callbacks := sub.Callbacks
		sub.mu.Unlock()

		// Notify all callbacks
		for _, cb := range callbacks {
			go cb(results)
		}
	} else {
		sub.mu.Lock()
		sub.LastResult = results
		callbacks := sub.Callbacks
		sub.mu.Unlock()

		for _, cb := range callbacks {
			go cb(results)
		}
	}
}

// hashResults creates a hash of the results for deduplication
func hashResults(results []map[string]interface{}) string {
	data, _ := json.Marshal(results)
	// Simple hash - in production use a proper hash function
	var hash uint64
	for _, b := range data {
		hash = hash*31 + uint64(b)
	}
	return fmt.Sprintf("%x", hash)
}

// SubscriptionInfo provides information about a subscription
type SubscriptionInfo struct {
	ID           string                 `json:"id"`
	Query        string                 `json:"query"`
	Variables    map[string]interface{} `json:"variables,omitempty"`
	Interval     string                 `json:"interval"`
	CreatedAt    time.Time              `json:"created_at"`
	LastPolledAt time.Time              `json:"last_polled_at"`
	ErrorCount   int                    `json:"error_count"`
	Active       bool                   `json:"active"`
}

// GetInfo returns information about the subscription
func (s *Subscription) GetInfo() SubscriptionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	return SubscriptionInfo{
		ID:           s.ID,
		Query:        s.Query,
		Variables:    s.Variables,
		Interval:     s.Interval.String(),
		CreatedAt:    s.CreatedAt,
		LastPolledAt: s.LastPolledAt,
		ErrorCount:   s.ErrorCount,
		Active:       s.Active,
	}
}
