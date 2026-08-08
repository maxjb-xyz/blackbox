package pm2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseJList(t *testing.T) {
	processes, err := parseJList([]byte(`[
    {"pm_id":2,"name":"api","pid":1234,"pm2_env":{"status":"online","restart_time":3,"unstable_restarts":1}},
    {"pm_id":5,"name":"worker","pid":0,"pm2_env":{"status":"stopped","restart_time":1}}
]`))
	require.NoError(t, err)
	require.Len(t, processes, 2)
	require.Equal(t, 2, processes[0].PMID)
	require.Equal(t, "api", processes[0].Name)
	require.Equal(t, "online", processes[0].PM2Env.Status)
	require.Equal(t, 3, processes[0].PM2Env.RestartTime)
}

func TestTransitionsEmitNormalizedLifecycleEntries(t *testing.T) {
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	previous := snapshot{
		"id:2": {PMID: 2, Name: "api", PID: 100, PM2Env: pm2Env("online", 1)},
		"id:3": {PMID: 3, Name: "worker", PID: 101, PM2Env: pm2Env("online", 0)},
	}
	current := []process{
		{PMID: 2, Name: "api", PID: 102, PM2Env: pm2Env("online", 2)},
		{PMID: 3, Name: "worker", PID: 0, PM2Env: pm2Env("errored", 0)},
		{PMID: 4, Name: "cron", PID: 103, PM2Env: pm2Env("online", 0)},
	}

	entries, next := transitions("node-1", now, previous, current, true, nil)
	require.Len(t, entries, 3)
	require.Len(t, next, 3)

	byService := make(map[string]string, len(entries))
	for _, entry := range entries {
		byService[entry.Service] = entry.Event
		require.Equal(t, "pm2", entry.Source)
		require.Equal(t, "node-1", entry.NodeName)
		require.Equal(t, now, entry.Timestamp)
		require.NotEmpty(t, entry.Content)
	}
	require.Equal(t, "restart", byService["api"])
	require.Equal(t, "failed", byService["worker"])
	require.Equal(t, "started", byService["cron"])
}

func TestTransitionsInitialPollIsBaselineAndProcessFilterIsExact(t *testing.T) {
	processes := []process{
		{PMID: 1, Name: "api", PM2Env: pm2Env("online", 0)},
		{PMID: 2, Name: "api-worker", PM2Env: pm2Env("online", 0)},
	}

	entries, next := transitions("node-1", time.Now(), nil, processes, false, []string{"api"})
	require.Empty(t, entries)
	require.Len(t, next, 1)

	entries, _ = transitions("node-1", time.Now(), next, []process{
		{PMID: 1, Name: "api", PM2Env: pm2Env("stopped", 0)},
		{PMID: 2, Name: "api-worker", PM2Env: pm2Env("errored", 1)},
	}, true, []string{"api"})
	require.Len(t, entries, 1)
	require.Equal(t, "stopped", entries[0].Event)
}

func pm2Env(status string, restartTime int) struct {
	Status           string `json:"status"`
	RestartTime      int    `json:"restart_time"`
	UnstableRestarts int    `json:"unstable_restarts"`
	PMUptime         int64  `json:"pm_uptime"`
	ExitCode         int    `json:"exit_code"`
} {
	return struct {
		Status           string `json:"status"`
		RestartTime      int    `json:"restart_time"`
		UnstableRestarts int    `json:"unstable_restarts"`
		PMUptime         int64  `json:"pm_uptime"`
		ExitCode         int    `json:"exit_code"`
	}{Status: status, RestartTime: restartTime}
}
