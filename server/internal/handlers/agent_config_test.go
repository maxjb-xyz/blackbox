package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAgentConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Node{}, &models.DataSourceInstance{}, &models.AppSetting{}, &models.SystemdUnitConfig{}))
	return db
}

func TestAgentConfig_ReadsFromDataSourceInstances(t *testing.T) {
	db := newAgentConfigTestDB(t)

	nodeName := "homelab-01"
	cfgJSON, _ := json.Marshal(map[string]any{"redact_secrets": false})
	db.Create(&models.DataSourceInstance{
		ID: "fw1", Type: "filewatcher", Scope: "agent", NodeID: &nodeName,
		Name: "File Watcher", Config: string(cfgJSON), Enabled: true,
	})
	unitsCfg, _ := json.Marshal(map[string]any{"units": []string{"nginx.service"}})
	db.Create(&models.DataSourceInstance{
		ID: "sys1", Type: "systemd", Scope: "agent", NodeID: &nodeName,
		Name: "Systemd", Config: string(unitsCfg), Enabled: true,
	})
	db.Create(&models.Node{ID: "n1", Name: nodeName, LastSeen: time.Now(), Capabilities: "[]"})

	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Node-Name", nodeName)
	req.Header.Set("X-Blackbox-Agent-Capabilities", "docker,systemd,filewatcher,proxmox")
	w := httptest.NewRecorder()
	handlers.AgentConfig(db)(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, false, resp["file_watcher_redact_secrets"])
	units := resp["systemd_units"].([]any)
	require.Equal(t, "nginx.service", units[0])

	// Capabilities stored on node
	var node models.Node
	require.NoError(t, db.Where("name = ?", nodeName).First(&node).Error)
	var caps []string
	require.NoError(t, json.Unmarshal([]byte(node.Capabilities), &caps))
	require.Contains(t, caps, "proxmox")
}
