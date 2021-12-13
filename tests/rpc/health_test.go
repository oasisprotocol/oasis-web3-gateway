package rpc

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHealthCheck(t *testing.T) {
	// Ensure the initial health-check was done.
	<-time.After(20 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), OasisBlockTimeout)
	defer cancel()
	endpoint, err := w3.GetHTTPEndpoint()
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/health", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "health request")
	defer resp.Body.Close()
	require.Equal(t, resp.StatusCode, http.StatusOK, "health check response")
}
