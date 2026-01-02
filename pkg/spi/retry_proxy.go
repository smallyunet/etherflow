package spi

import (
	"context"

	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/util"
)

// RetryingBlockSource wraps a BlockSource with retry logic
type RetryingBlockSource struct {
	inner   BlockSource
	backoff *util.Backoff
}

// NewRetryingBlockSource creates a new RetryingBlockSource
func NewRetryingBlockSource(inner BlockSource, backoff *util.Backoff) *RetryingBlockSource {
	return &RetryingBlockSource{
		inner:   inner,
		backoff: backoff,
	}
}

// Inner returns the underlying BlockSource
func (s *RetryingBlockSource) Inner() BlockSource {
	return s.inner
}

// LatestBlock returns the latest block number and hash from the node with retry
func (s *RetryingBlockSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	var number uint64
	var hash core.Hash

	err := s.backoff.Retry(ctx, func() error {
		var err error
		number, hash, err = s.inner.LatestBlock(ctx)
		return err
	})

	return number, hash, err
}

// GetBlockByNumber fetches a block by its number with retry
func (s *RetryingBlockSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	var block *core.Block

	err := s.backoff.Retry(ctx, func() error {
		var err error
		block, err = s.inner.GetBlockByNumber(ctx, number)
		return err
	})

	return block, err
}

// GetBlockByHash fetches a block by its hash with retry
func (s *RetryingBlockSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	var block *core.Block

	err := s.backoff.Retry(ctx, func() error {
		var err error
		block, err = s.inner.GetBlockByHash(ctx, hash)
		return err
	})

	return block, err
}
