package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const redisStream = "newmcp:virtual:changes"

// Each instance reads the same stream with its own cursor. Redis Streams retain
// changes across a temporary disconnect, and BLOCK 0 waits without polling.
type redisChange struct {
	Origin            string `json:"origin"`
	Type              string `json:"type"`
	Kind              string `json:"kind"`
	UserID            int64  `json:"user_id,omitempty"`
	ServiceID         int64  `json:"service_id,omitempty"`
	MarketplaceItemID int64  `json:"marketplace_item_id,omitempty"`
	ConfigID          int64  `json:"config_id"`
}

type redisBridge struct {
	client  *redis.Client
	ctx     context.Context
	cancel  context.CancelFunc
	origin  string
	wake    chan struct{}
	mu      sync.Mutex
	pending map[redisChange]struct{}
	workers sync.WaitGroup
}

var activeRedis atomic.Pointer[redisBridge]

// StartRedis enables cross-instance virtual-service notifications. An empty URL
// keeps single-process notifications only. A configured but unavailable Redis
// fails startup, since multi-instance changes would otherwise be missed.
func StartRedis(ctx context.Context, url string) (func(), error) {
	if strings.TrimSpace(url) == "" {
		return func() {}, nil
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse Redis connection string: %w", err)
	}
	options.ContextTimeoutEnabled = true
	// Surface transport errors to the reader so it can reconcile listeners on
	// reconnect; the bridge itself retries failed reads and queued writes.
	options.MaxRetries = -1
	client := redis.NewClient(options)
	checkCtx, cancelCheck := context.WithTimeout(ctx, 3*time.Second)
	defer cancelCheck()
	if err := client.Ping(checkCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to Redis for virtual-service events: %w", err)
	}
	latest, err := client.XRevRangeN(checkCtx, redisStream, "+", "-", 1).Result()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("read virtual-service event cursor: %w", err)
	}
	cursor := "0-0"
	if len(latest) != 0 {
		cursor = latest[0].ID
	}
	bridgeCtx, cancel := context.WithCancel(ctx)
	b := &redisBridge{
		client: client, ctx: bridgeCtx, cancel: cancel,
		origin: uuid.NewString(), wake: make(chan struct{}, 1),
		pending: make(map[redisChange]struct{}),
	}
	if !activeRedis.CompareAndSwap(nil, b) {
		cancel()
		_ = client.Close()
		return nil, fmt.Errorf("Redis virtual-service events are already running")
	}
	b.workers.Add(2)
	go b.read(cursor)
	go b.publish()
	var once sync.Once
	return func() {
		once.Do(func() {
			activeRedis.CompareAndSwap(b, nil)
			cancel()
			_ = client.Close()
			b.workers.Wait()
		})
	}, nil
}

func (b *redisBridge) enqueue(change redisChange) {
	change.Origin = b.origin
	b.mu.Lock()
	b.pending[change] = struct{}{}
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

func (b *redisBridge) pop() (redisChange, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for change := range b.pending {
		delete(b.pending, change)
		return change, true
	}
	return redisChange{}, false
}

func (b *redisBridge) requeue(change redisChange) {
	b.mu.Lock()
	b.pending[change] = struct{}{}
	b.mu.Unlock()
}

func (b *redisBridge) publish() {
	defer b.workers.Done()
	failed := false
	for {
		if b.ctx.Err() != nil {
			return
		}
		change, ok := b.pop()
		if !ok {
			select {
			case <-b.ctx.Done():
				return
			case <-b.wake:
				continue
			}
		}
		payload, _ := json.Marshal(change)
		writeCtx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
		err := b.client.XAdd(writeCtx, &redis.XAddArgs{
			Stream: redisStream, MaxLen: 100000, Approx: true,
			Values: map[string]any{"payload": string(payload)},
		}).Err()
		cancel()
		if err != nil {
			b.requeue(change)
			if b.ctx.Err() != nil {
				return
			}
			if !failed {
				log.Printf("Redis virtual-service event publish failed: %v", err)
				failed = true
			}
			if !b.retry() {
				return
			}
			continue
		}
		if failed {
			log.Printf("Redis virtual-service event publishing recovered")
			failed = false
		}
	}
}

func (b *redisBridge) read(cursor string) {
	defer b.workers.Done()
	reconnect := false
	for {
		if b.ctx.Err() != nil {
			return
		}
		if reconnect {
			checkCtx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
			err := b.client.Ping(checkCtx).Err()
			cancel()
			if err != nil {
				if !b.retry() {
					return
				}
				continue
			}
			// Recheck all active adapters once in case this instance missed
			// changes while Redis was disconnected or its stream was trimmed.
			catchUpAll()
			reconnect = false
		}
		streams, err := b.client.XRead(b.ctx, &redis.XReadArgs{
			Streams: []string{redisStream, cursor}, Count: 128, Block: 0,
		}).Result()
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			log.Printf("Redis virtual-service event read failed: %v", err)
			reconnect = true
			if !b.retry() {
				return
			}
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				cursor = message.ID
				payload, ok := message.Values["payload"].(string)
				if !ok {
					continue
				}
				var change redisChange
				if json.Unmarshal([]byte(payload), &change) != nil || change.Origin == b.origin {
					continue
				}
				switch change.Type {
				case "task":
					dispatchTask(change.Kind, change.UserID, change.ServiceID, change.ConfigID)
				case "config":
					dispatchConfig(change.Kind, change.ConfigID)
				case "service":
					dispatchService(change.ServiceID)
				case "marketplace":
					dispatchMarketplace(change.MarketplaceItemID)
				}
			}
		}
	}
}

func (b *redisBridge) retry() bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-b.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
