package spi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/util"
)

// MockFailingSource is a mock BlockSource that fails n times before succeeding
type MockFailingSource struct {
	FailCount    int
	CurrentFails int
}

func (m *MockFailingSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	if m.CurrentFails < m.FailCount {
		m.CurrentFails++
		return 0, "", errors.New("simulated error")
	}
	return 100, "0x100", nil
}

func (m *MockFailingSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	if m.CurrentFails < m.FailCount {
		m.CurrentFails++
		return nil, errors.New("simulated error")
	}
	return &core.Block{Number: number}, nil
}

func (m *MockFailingSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	if m.CurrentFails < m.FailCount {
		m.CurrentFails++
		return nil, errors.New("simulated error")
	}
	return &core.Block{Hash: hash}, nil
}

func TestRetryingBlockSource_LatestBlock(t *testing.T) {
	mock := &MockFailingSource{FailCount: 2}
	// fast backoff for test
	backoff := util.NewBackoff(3, 1*time.Millisecond)

	proxy := NewRetryingBlockSource(mock, backoff)

	_, _, err := proxy.LatestBlock(context.Background())
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if mock.CurrentFails != 2 {
		t.Errorf("expected 2 failures, got %d", mock.CurrentFails)
	}
}

func TestRetryingBlockSource_FailEventually(t *testing.T) {
	mock := &MockFailingSource{FailCount: 5}
	// backoff only retries 3 times
	backoff := util.NewBackoff(3, 1*time.Millisecond)

	proxy := NewRetryingBlockSource(mock, backoff)

	_, _, err := proxy.LatestBlock(context.Background())
	if err == nil {
		t.Fatal("expected error, got success")
	}
}
