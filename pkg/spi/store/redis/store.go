package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/spi"
)

type Store struct {
	client *redis.Client
	prefix string
}

// Ensure Store implements spi.StateStore
var _ spi.StateStore = (*Store)(nil)

func NewStore(addr string, password string, db int) (*Store, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Store{
		client: rdb,
		prefix: "etherflow:",
	}, nil
}

func (s *Store) GetLastBlock(ctx context.Context) (*core.Block, error) {
	// key: etherflow:head
	val, err := s.client.Get(ctx, s.prefix+"head").Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var block core.Block
	if err := json.Unmarshal([]byte(val), &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}
	return &block, nil
}

func (s *Store) SaveBlock(ctx context.Context, block *core.Block) error {
	data, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %w", err)
	}

	pipe := s.client.TxPipeline()

	// 1. Save block data by number
	// key: etherflow:block:<number>
	blockKey := fmt.Sprintf("%sblock:%d", s.prefix, block.Number)
	pipe.Set(ctx, blockKey, data, 0)

	// 2. Update head
	// key: etherflow:head
	pipe.Set(ctx, s.prefix+"head", data, 0)

	// 3. Save hash mapping (optional but good for lookups)
	// key: etherflow:hash:<hash> -> number
	// pipe.Set(ctx, s.prefix+"hash:"+string(block.Hash), block.Number, 0)

	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	blockKey := fmt.Sprintf("%sblock:%d", s.prefix, number)
	val, err := s.client.Get(ctx, blockKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var block core.Block
	if err := json.Unmarshal([]byte(val), &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}
	return &block, nil
}

// Rewind deletes blocks from (number + 1) to head
// This is trickier in KV store if we don't track the exact head number separately easier.
// But we do have GetLastBlock.
func (s *Store) Rewind(ctx context.Context, toBlock uint64) error {
	head, err := s.GetLastBlock(ctx)
	if err != nil {
		return err
	}
	if head == nil {
		return nil
	}

	if head.Number <= toBlock {
		return nil
	}

	// We need to delete all blocks > toBlock
	// In Redis, we can just update the head.
	// The old blocks (orphans) will remain but will be overwritten if the chain advances again.
	// To be cleaner, we should delete them, but for performance, overwriting/ignoring might be acceptable
	// IF we trust 'head' as the source of truth for "where are we".
	// HOWEVER, if we used `GetBlockByNumber` for history checks, we might read orphaned blocks if we don't delete them.
	// So we MUST delete them or handle overwrites carefully.

	// Let's delete them to be safe.
	var keys []string
	for i := head.Number; i > toBlock; i-- {
		keys = append(keys, fmt.Sprintf("%sblock:%d", s.prefix, i))
	}

	if len(keys) > 0 {
		if err := s.client.Del(ctx, keys...).Err(); err != nil {
			return err
		}
	}

	// Determine the new head
	// It should be the block at `toBlock`
	newHeadBlock, err := s.GetBlockByNumber(ctx, toBlock)
	if err != nil {
		return err
	}
	if newHeadBlock == nil {
		// This is bad. We are rewinding to a block we don't have?
		// Maybe allow it?
		return fmt.Errorf("rewind target block %d not found", toBlock)
	}

	return s.SaveBlock(ctx, newHeadBlock)
}
