package oasis

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
)

// errKeyManager is a stand-in for the key manager failures that the health checker should tolerate.
var errKeyManager = errors.New("key manager unavailable")

func newTestHealthChecker() *healthChecker {
	return &healthChecker{logger: logging.GetLogger("oasis_test")}
}

func TestHealthCheckerToleratesShortFailures(t *testing.T) {
	require := require.New(t)

	h := newTestHealthChecker()
	h.recordResult(nil)
	require.NoError(h.Health(), "should be healthy after a successful iteration")

	// Fewer than healthUnhealthyThreshold consecutive failures must not flip the health state.
	for i := 1; i < healthUnhealthyThreshold; i++ {
		h.recordResult(errKeyManager)
		require.NoError(h.Health(), "should still be healthy after %d consecutive failures", i)
	}

	// A success in between clears the counter.
	h.recordResult(nil)
	require.NoError(h.Health(), "should be healthy after recovering")

	for i := 1; i < healthUnhealthyThreshold; i++ {
		h.recordResult(errKeyManager)
		require.NoError(h.Health(), "counter should have been reset by the successful iteration")
	}
}

func TestHealthCheckerReportsSustainedFailures(t *testing.T) {
	require := require.New(t)

	h := newTestHealthChecker()
	h.recordResult(nil)

	for range healthUnhealthyThreshold {
		h.recordResult(errKeyManager)
	}
	require.Error(h.Health(), "should be unhealthy after reaching the threshold")

	// Every further failure keeps it unhealthy.
	h.recordResult(errKeyManager)
	require.Error(h.Health(), "should stay unhealthy while failures continue")

	// A single success is enough to recover.
	h.recordResult(nil)
	require.NoError(h.Health(), "should recover on the first successful iteration")
}

func TestHealthCheckerRetriesFasterWhileFailing(t *testing.T) {
	require := require.New(t)

	h := newTestHealthChecker()
	h.recordResult(nil)
	require.Equal(healthCheckInterval, h.nextInterval(), "should use the normal interval while healthy")

	// Probe more often from the very first failure, so that it is confirmed or cleared quickly.
	h.recordResult(errKeyManager)
	require.Equal(healthRetryInterval, h.nextInterval(), "should retry faster after a failure")

	for range healthUnhealthyThreshold {
		h.recordResult(errKeyManager)
	}
	require.Error(h.Health())
	require.Equal(healthRetryInterval, h.nextInterval(), "should keep retrying faster while unhealthy")

	h.recordResult(nil)
	require.Equal(healthCheckInterval, h.nextInterval(), "should return to the normal interval once recovered")
}

func TestHealthCheckerUnhealthyUntilFirstSuccess(t *testing.T) {
	require := require.New(t)

	h := newTestHealthChecker()
	require.Error(h.Health(), "should be unhealthy before the first iteration")

	h.recordResult(errKeyManager)
	require.Error(h.Health(), "should stay unhealthy while it has never been healthy")

	h.recordResult(nil)
	require.NoError(h.Health(), "should become healthy after the first successful iteration")
}
