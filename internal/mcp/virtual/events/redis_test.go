package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/mujkjk/newmcp/model"
	"github.com/redis/go-redis/v9"
)

func TestRedisBridgePublishesAndReceivesAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := StartRedis(ctx, "redis://"+server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	sub := Subscribe(model.VirtualExecution{Kind: "imagegen", UserID: 2, ServiceID: 20, ConfigID: 4})
	defer sub.Close()

	TaskChanged("imagegen", 2, 20, 4)
	waitEvent(t, sub)
	if task, config := sub.Drain(); !task || config {
		t.Fatalf("local task flags = %t/%t", task, config)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		count, err := client.XLen(ctx, redisStream).Result()
		if err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("local change was not published to Redis")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A different origin represents another backend process. Its event is a
	// read barrier after our own event, so the flag check also detects echoes.
	payload, _ := json.Marshal(redisChange{Origin: "other-instance", Type: "config", Kind: "imagegen", ConfigID: 4})
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: redisStream, Values: map[string]any{"payload": string(payload)},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, sub)
	if task, config := sub.Drain(); task || !config {
		t.Fatalf("remote config flags = %t/%t", task, config)
	}
	serviceInvalidated := make(chan int64, 1)
	marketInvalidated := make(chan int64, 1)
	RegisterInvalidationHandlers(func(id int64) { serviceInvalidated <- id }, func(id int64) { marketInvalidated <- id })
	defer RegisterInvalidationHandlers(nil, nil)
	for _, change := range []redisChange{
		{Origin: "other-instance", Type: "service", ServiceID: 99},
		{Origin: "other-instance", Type: "marketplace", MarketplaceItemID: 33},
	} {
		payload, _ := json.Marshal(change)
		if err := client.XAdd(ctx, &redis.XAddArgs{
			Stream: redisStream, Values: map[string]any{"payload": string(payload)},
		}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case id := <-serviceInvalidated:
		if id != 99 {
			t.Fatalf("wrong service invalidated: %d", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("remote service invalidation was not delivered")
	}
	select {
	case id := <-marketInvalidated:
		if id != 33 {
			t.Fatalf("wrong marketplace item invalidated: %d", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("remote marketplace invalidation was not delivered")
	}
}

func waitEvent(t *testing.T, sub *Subscription) {
	t.Helper()
	select {
	case <-sub.Ready():
	case <-time.After(3 * time.Second):
		t.Fatal("virtual-service event was not delivered")
	}
}

func TestRedisBridgeCatchesUpAfterReconnect(t *testing.T) {
	server := miniredis.RunT(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := StartRedis(ctx, "redis://"+server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	sub := Subscribe(model.VirtualExecution{Kind: "imagegen", UserID: 2, ServiceID: 20, ConfigID: 4})
	defer sub.Close()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	payload, _ := json.Marshal(redisChange{Origin: "other-instance", Type: "task", Kind: "imagegen", UserID: 2, ServiceID: 20, ConfigID: 4})
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: redisStream, Values: map[string]any{"payload": string(payload)}}).Err(); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, sub)
	sub.Drain()
	server.Close()
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sub.Ready():
		if task, config := sub.Drain(); !task || !config {
			t.Fatalf("reconnect catch-up flags = %t/%t", task, config)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("virtual-service listeners were not resynchronized after Redis reconnect")
	}
}
