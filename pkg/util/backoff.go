package util

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Backoff implements exponential backoff
type Backoff struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// NewBackoff creates a new Backoff instance
func NewBackoff(maxRetries int, baseDelay time.Duration) *Backoff {
	return &Backoff{
		MaxRetries: maxRetries,
		BaseDelay:  baseDelay,
		MaxDelay:   30 * time.Second,
	}
}

// Retry executes the operation with exponential backoff
func (b *Backoff) Retry(ctx context.Context, op func() error) error {
	var err error
	for i := 0; i <= b.MaxRetries; i++ {
		if err = op(); err == nil {
			return nil
		}

		if i == b.MaxRetries {
			break
		}

		delay := time.Duration(math.Pow(2, float64(i))) * b.BaseDelay
		if delay > b.MaxDelay {
			delay = b.MaxDelay
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			fmt.Printf("Retrying after error: %v. Attempt %d/%d\n", err, i+1, b.MaxRetries)
		}
	}
	return fmt.Errorf("operation failed after %d retries: %w", b.MaxRetries, err)
}
