package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	logStreamPrefix = "nova:logs:"
	logStreamTTL    = 24 * time.Hour // Keep logs for 24 hours
	maxLogEntries   = 10000          // Max entries per function
)

// Entry represents a log entry
type Entry struct {
	Timestamp  time.Time `json:"timestamp"`
	RequestID  string    `json:"request_id"`
	FunctionID string    `json:"function_id"`
	Function   string    `json:"function"`
	Level      string    `json:"level"`
	Message    string    `json:"message"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	ColdStart  bool      `json:"cold_start,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// Store manages function logs in Redis Streams
type Store struct {
	redis *redis.Client
}

// NewStore creates a new log store
func NewStore(redis *redis.Client) *Store {
	return &Store{redis: redis}
}

// Append adds a log entry to the function's log stream
func (s *Store) Append(ctx context.Context, entry Entry) error {
	key := logStreamPrefix + entry.FunctionID

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Add to stream with auto-generated ID
	_, err = s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: maxLogEntries,
		Approx: true,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("xadd: %w", err)
	}

	// Set TTL on first entry
	s.redis.Expire(ctx, key, logStreamTTL)

	return nil
}

// Query retrieves log entries for a function
type QueryOptions struct {
	FunctionID string
	Since      time.Time
	Until      time.Time
	Limit      int64
	RequestID  string // Filter by request ID
}

// Query retrieves log entries based on options
func (s *Store) Query(ctx context.Context, opts QueryOptions) ([]Entry, error) {
	key := logStreamPrefix + opts.FunctionID

	start := "-"
	end := "+"

	if !opts.Since.IsZero() {
		start = fmt.Sprintf("%d", opts.Since.UnixMilli())
	}
	if !opts.Until.IsZero() {
		end = fmt.Sprintf("%d", opts.Until.UnixMilli())
	}

	count := opts.Limit
	if count == 0 {
		count = 100
	}

	messages, err := s.redis.XRange(ctx, key, start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("xrange: %w", err)
	}

	var entries []Entry
	for _, msg := range messages {
		data, ok := msg.Values["data"].(string)
		if !ok {
			continue
		}

		var entry Entry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			continue
		}

		// Filter by request ID if specified
		if opts.RequestID != "" && entry.RequestID != opts.RequestID {
			continue
		}

		entries = append(entries, entry)

		if int64(len(entries)) >= count {
			break
		}
	}

	return entries, nil
}

// Tail returns a channel that receives new log entries in real-time
func (s *Store) Tail(ctx context.Context, functionID string) (<-chan Entry, error) {
	key := logStreamPrefix + functionID
	ch := make(chan Entry, 100)

	go func() {
		defer close(ch)

		// Start from the latest entry
		lastID := "$"

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Block for up to 1 second waiting for new entries
			streams, err := s.redis.XRead(ctx, &redis.XReadArgs{
				Streams: []string{key, lastID},
				Count:   100,
				Block:   time.Second,
			}).Result()

			if err == redis.Nil {
				continue
			}
			if err != nil {
				// Context cancelled or other error
				return
			}

			for _, stream := range streams {
				for _, msg := range stream.Messages {
					lastID = msg.ID

					data, ok := msg.Values["data"].(string)
					if !ok {
						continue
					}

					var entry Entry
					if err := json.Unmarshal([]byte(data), &entry); err != nil {
						continue
					}

					select {
					case ch <- entry:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, nil
}

// Recent returns the most recent log entries for a function
func (s *Store) Recent(ctx context.Context, functionID string, count int64) ([]Entry, error) {
	key := logStreamPrefix + functionID

	if count == 0 {
		count = 50
	}

	messages, err := s.redis.XRevRange(ctx, key, "+", "-").Result()
	if err != nil {
		return nil, fmt.Errorf("xrevrange: %w", err)
	}

	var entries []Entry
	for i := len(messages) - 1; i >= 0 && int64(len(entries)) < count; i-- {
		msg := messages[i]
		data, ok := msg.Values["data"].(string)
		if !ok {
			continue
		}

		var entry Entry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// Clear removes all logs for a function
func (s *Store) Clear(ctx context.Context, functionID string) error {
	key := logStreamPrefix + functionID
	return s.redis.Del(ctx, key).Err()
}
