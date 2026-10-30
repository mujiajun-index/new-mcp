package service

import "github.com/mujkjk/newmcp/internal/mcp/virtual/events"

func init() {
	events.RegisterInvalidationHandlers(
		func(serviceID int64) {
			if SessionPool != nil {
				SessionPool.Remove(serviceID)
			}
		},
		func(itemID int64) {
			if SessionPool != nil {
				SessionPool.RemoveByMarketplaceItem(itemID)
			}
		},
	)
}
