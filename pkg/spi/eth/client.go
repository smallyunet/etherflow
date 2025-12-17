package eth

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/username/etherflow/pkg/core"
)

// Client implements spi.BlockSource using go-ethereum's ethclient
type Client struct {
	rpc *ethclient.Client
}

// NewClient creates a new Client connected to the given URL
func NewClient(rawurl string) (*Client, error) {
	rpc, err := ethclient.Dial(rawurl)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", rawurl, err)
	}
	return &Client{rpc: rpc}, nil
}

// LatestBlock returns the latest block number and hash from the node
func (c *Client) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	header, err := c.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	return header.Number.Uint64(), core.Hash(header.Hash().Hex()), nil
}

// GetBlockByNumber fetches a block by its number
func (c *Client) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	bigNum := new(big.Int).SetUint64(number)
	ethBlock, err := c.rpc.BlockByNumber(ctx, bigNum)
	if err != nil {
		return nil, err
	}
	return c.enrichBlock(ctx, ethBlock)
}

// GetBlockByHash fetches a block by its hash
func (c *Client) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	ethHash := common.HexToHash(string(hash))
	ethBlock, err := c.rpc.BlockByHash(ctx, ethHash)
	if err != nil {
		return nil, err
	}
	return c.enrichBlock(ctx, ethBlock)
}

// enrichBlock takes a geth block, fetches its logs, and returns an etherflow core.Block
func (c *Client) enrichBlock(ctx context.Context, ethBlock *types.Block) (*core.Block, error) {
	hash := ethBlock.Hash()
	
	// Fetch logs for this block
	query := ethereum.FilterQuery{
		BlockHash: &hash,
	}
	
	logs, err := c.rpc.FilterLogs(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs for block %s: %w", hash.Hex(), err)
	}
	
	coreLogs := make([]core.Log, len(logs))
	for i, l := range logs {
		coreLogs[i] = core.Log{
			Address:     core.Address(l.Address.Hex()),
			Topics:      make([]core.Hash, len(l.Topics)),
			Data:        l.Data,
			BlockNumber: l.BlockNumber,
			TxHash:      core.Hash(l.TxHash.Hex()),
			Index:       l.Index,
		}
		for j, t := range l.Topics {
			coreLogs[i].Topics[j] = core.Hash(t.Hex())
		}
	}

	return &core.Block{
		Number:     ethBlock.NumberU64(),
		Hash:       core.Hash(ethBlock.Hash().Hex()),
		ParentHash: core.Hash(ethBlock.ParentHash().Hex()),
		Timestamp:  ethBlock.Time(),
		Logs:       coreLogs,
	}, nil
}
