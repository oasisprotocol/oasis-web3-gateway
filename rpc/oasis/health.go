package oasis

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-web3-gateway/source"
)

const (
	healthCheckInterval    = 30 * time.Second
	healthIterationTimeout = 15 * time.Second

	// healthRetryInterval is the interval between iterations while the previous one failed. It is
	// shorter than healthCheckInterval so that a short-lived failure is confirmed or cleared
	// quickly, both when going unhealthy and when recovering.
	healthRetryInterval = 5 * time.Second

	// healthUnhealthyThreshold is the number of consecutive failed iterations after which the
	// service is reported as unhealthy. Querying the public key can fail for a short while (e.g.
	// when the key manager is momentarily unreachable), and a single such failure should not take
	// the gateway out of rotation. Since failing iterations are retried on healthRetryInterval,
	// this tolerance delays reporting a genuine outage by roughly
	// (healthUnhealthyThreshold-1)*healthRetryInterval.
	healthUnhealthyThreshold = 3
)

type healthChecker struct {
	ctx    context.Context
	source source.NodeSource
	logger *logging.Logger

	health uint32

	// failures is the number of consecutive failed iterations. Only accessed from the run goroutine.
	failures uint
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

// recordResult folds the result of a single health check iteration into the health state.
func (h *healthChecker) recordResult(err error) {
	if err == nil {
		h.failures = 0
		h.logger.Debug("oasis_ RPC healthy")
		h.updateHealth(true)
		return
	}

	h.failures++
	if h.failures < healthUnhealthyThreshold {
		h.logger.Warn("failed to fetch public key, tolerating",
			"err", err,
			"failures", h.failures,
			"threshold", healthUnhealthyThreshold,
		)
		return
	}

	h.logger.Error("failed to fetch public key",
		"err", err,
		"failures", h.failures,
	)
	h.updateHealth(false)
}

// nextInterval returns the delay before the next iteration. While iterations are failing the
// checker probes more often, so that the failure is confirmed or cleared quickly.
func (h *healthChecker) nextInterval() time.Duration {
	if h.failures > 0 {
		return healthRetryInterval
	}
	return healthCheckInterval
}

func (h *healthChecker) run() {
	for {
		select {
		case <-time.After(h.nextInterval()):
			func() {
				ctx, cancel := context.WithTimeout(h.ctx, healthIterationTimeout)
				defer cancel()

				// Query public keys.
				_, err := h.source.CoreCallDataPublicKey(ctx)
				h.recordResult(err)
			}()
		case <-h.ctx.Done():
			h.updateHealth(false)
			h.logger.Debug("health checker stopping", "reason", h.ctx.Err())
			return
		}
	}
}
