package oasis

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/core"
)

const (
	healthCheckInterval    = 30 * time.Second
	healthIterationTimeout = 15 * time.Second
)

type healthChecker struct {
	ctx    context.Context
	client client.RuntimeClient
	logger *logging.Logger

	health uint32
}

// Implements server.HealthCheck.
func (h *healthChecker) Health() error {
	if atomic.LoadUint32(&h.health) == 0 {
		return fmt.Errorf("oasis API not healthy")
	}
	return nil
}

func (h *healthChecker) updateHealth(healthy bool) {
	if healthy {
		atomic.StoreUint32(&h.health, 1)
	} else {
		atomic.StoreUint32(&h.health, 0)
	}
}

func (h *healthChecker) run() {
	for {
		select {
		case <-time.After(healthCheckInterval):
			func() {
				ctx, cancel := context.WithTimeout(h.ctx, healthIterationTimeout)
				defer cancel()

				// Query public keys.
				_, err := core.NewV1(h.client).CallDataPublicKey(ctx)
				if err != nil {
					h.logger.Error("failed to fetch public key", "err", err)
					h.updateHealth(false)
					return
				}

				h.logger.Debug("oasis_ RPC healthy")
				h.updateHealth(true)
			}()
		case <-h.ctx.Done():
			h.updateHealth(false)
			h.logger.Debug("health checker stopping", "reason", h.ctx.Err())
			return
		}
	}
}
