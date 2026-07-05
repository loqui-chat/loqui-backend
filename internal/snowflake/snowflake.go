// Package snowflake generates 64 bit time sortable ids
//
// layout: 1 unused | 41ms since epoch | 10 node | 12 sequence
// timestamp is in high bits so id order is creation order
package snowflake

import (
	"errors"
	"sync"
	"time"
)

// Epoch is project zero point in unix ms (2026-01-01)
// !!DO NOT CHANGE!!
const Epoch int64 = 1767225600000

const (
	nodeBits = 10
	seqBits  = 12

	maxNode = -1 ^ (-1 << nodeBits)
	maxSeq  = -1 ^ (-1 << seqBits)

	timeShift = nodeBits + seqBits
	nodeShift = seqBits
)

var (
	ErrNodeRange      = errors.New("snowflake: node id out of rnage 0..1023")
	ErrClockBackwards = errors.New("snowflake: clock moved backwards")
)

// Generator is a single node id source
type Generator struct {
	mu       sync.Mutex
	node     int64
	lastMs   int64
	sequence int64
}

// New builds a Generator
// A single intsance can pass node 0
func New(node int64) (*Generator, error) {
	if node < 0 || node > maxNode {
		return nil, ErrNodeRange
	}
	return &Generator{node: node}, nil
}

// Next returns next id
func (g *Generator) Next() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := nowMs()

	if now < g.lastMs {
		if g.lastMs-now > 5 {
			return 0, ErrClockBackwards
		}
		now = waitUntil(g.lastMs) // small drift, wait out
	}

	if now == g.lastMs {
		g.sequence = (g.sequence + 1) & maxSeq
		if g.sequence == 0 {
			now = waitUntil(g.lastMs + 1) // sequence exhausted this ms
		}
	} else {
		g.sequence = 0
	}

	g.lastMs = now
	return ((now - Epoch) << timeShift) | (g.node << nodeShift) | g.sequence, nil
}

func TimeOf(id int64) time.Time { return time.UnixMilli((id >> timeShift) + Epoch).UTC() }
func NodeOf(id int64) int64     { return (id >> nodeShift) & maxNode }
func SeqOf(id int64) int64      { return id & maxSeq }

func nowMs() int64 { return time.Now().UnixMilli() }

func waitUntil(target int64) int64 {
	now := nowMs()
	for now < target {
		time.Sleep(time.Duration(target-now) * time.Millisecond)
		now = nowMs()
	}
	return now
}
