package core

import (
	"context"
)

// Hash represents a 32-byte hash
type Hash string

// Address represents a 20-byte address
type Address string

// Block represents a simplified block header and data required for indexing
type Block struct {
	Number     uint64
	Hash       Hash
	ParentHash Hash
	Timestamp  uint64
	Logs       []Log
}

// Head represents a block header event from a subscription
type Head struct {
	Number     uint64
	Hash       Hash
	ParentHash Hash
	Timestamp  uint64
}

// Log represents an EVM log event
type Log struct {
	Address     Address
	Topics      []Hash
	Data        []byte
	BlockNumber uint64
	TxHash      Hash
	Index       uint
}

// EventContext provides context to the handler
type EventContext struct {
	Context context.Context
	Block   *Block
	Log     *Log
	IsReorg bool // True if this event is being replayed or handled during a reorg resolution
}

// Handler is the user-defined callback for events
type Handler func(ctx *EventContext) error

// ReorgHandler is called when a chain reorganization is detected
type ReorgHandler func(ctx context.Context, forkPoint *Block, oldChain []*Block, newChain []*Block) error
