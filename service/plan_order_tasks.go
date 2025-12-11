package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// StartPlanOrderTasks starts background tasks for plan order management
func StartPlanOrderTasks() {
	common.SysLog("starting plan order background tasks")

	// Start order expiration task (runs every 5 minutes)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			err := model.ExpireOldOrders()
			if err != nil {
				common.SysLog("order expiration task failed: " + err.Error())
			}
		}
	}()

	// Start delivery retry task (runs every 1 minute)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			RetryFailedDeliveries()
		}
	}()

	common.SysLog("plan order background tasks started")
}
