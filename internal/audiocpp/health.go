package audiocpp

import (
	"context"
	"fmt"
	"net"
	"time"
)

type ServerStatus struct {
	Alive    bool     `json:"alive"`
	Backend  string   `json:"backend"`
	ModelIDs []string `json:"model_ids"`
	Error    string   `json:"error,omitempty"`
}

func CheckServerHealth(ctx context.Context, host string, port int) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return MapError(err)
	}
	conn.Close()
	return nil
}

func WaitForServer(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return MapError(ctx.Err())
		default:
		}

		if err := CheckServerHealth(ctx, host, port); err == nil {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}
	return NewError(ErrServerUnavailable, "server did not become healthy within timeout", nil)
}

func GetServerStatus(ctx context.Context, client *Client) *ServerStatus {
	status := &ServerStatus{}

	healthResp, err := client.Health(ctx)
	if err != nil {
		status.Alive = false
		status.Error = err.Error()
		return status
	}

	status.Alive = true
	status.Backend = healthResp.Backend

	modelsResp, err := client.ListModels(ctx)
	if err == nil {
		status.ModelIDs = make([]string, len(modelsResp.Data))
		for i, m := range modelsResp.Data {
			status.ModelIDs[i] = m.ID
		}
	}

	return status
}
