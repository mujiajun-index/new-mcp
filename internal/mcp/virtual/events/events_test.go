package events

import (
	"testing"

	"github.com/mujkjk/newmcp/model"
)

func TestVirtualChangesStayInTheirServiceScope(t *testing.T) {
	itemID := int64(7)
	owner := Subscribe(model.VirtualExecution{UserID: 1, OwnerID: 1, ConfigID: 4, ServiceID: 10, Kind: "imagegen"})
	market := Subscribe(model.VirtualExecution{UserID: 2, OwnerID: 1, ConfigID: 4, ServiceID: 20, Kind: "imagegen", MarketplaceItemID: &itemID})
	other := Subscribe(model.VirtualExecution{UserID: 3, OwnerID: 1, ConfigID: 4, ServiceID: 30, Kind: "imagegen", MarketplaceItemID: &itemID})
	defer owner.Close()
	defer market.Close()
	defer other.Close()

	TaskChanged("imagegen", 2, 20, 4)
	select {
	case <-market.Ready():
	default:
		t.Fatal("market task change was not delivered")
	}
	if tasks, config := market.Drain(); !tasks || config {
		t.Fatalf("unexpected market change flags: %t/%t", tasks, config)
	}
	select {
	case <-owner.Ready():
		t.Fatal("platform owner received a buyer task")
	default:
	}
	select {
	case <-other.Ready():
		t.Fatal("another buyer received a task")
	default:
	}

	TaskChanged("imagegen", 1, 0, 4)
	select {
	case <-owner.Ready():
	default:
		t.Fatal("legacy owner task change was not delivered")
	}
	owner.Drain()
	select {
	case <-market.Ready():
		t.Fatal("legacy task leaked to marketplace installation")
	default:
	}

	ConfigChanged("imagegen", 4)
	for _, sub := range []*Subscription{owner, market, other} {
		select {
		case <-sub.Ready():
		default:
			t.Fatal("configuration change was not delivered")
		}
		if tasks, config := sub.Drain(); tasks || !config {
			t.Fatalf("unexpected configuration flags: %t/%t", tasks, config)
		}
	}
}
