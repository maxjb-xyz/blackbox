package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// MigrateDataSources seeds data_source_instances from legacy tables.
// Safe to call on every startup — skips rows that already exist.
func MigrateDataSources(db *gorm.DB, envWebhookSecret string) error {
	now := time.Now().UTC()

	// 1. Systemd: one instance per node in systemd_unit_configs
	var systemdConfigs []models.SystemdUnitConfig
	if err := db.Find(&systemdConfigs).Error; err != nil {
		return fmt.Errorf("load systemd configs: %w", err)
	}
	for _, cfg := range systemdConfigs {
		nodeName := cfg.NodeName
		var units []string
		if err := json.Unmarshal([]byte(cfg.Units), &units); err != nil {
			units = []string{}
		}
		cfgJSON, _ := json.Marshal(map[string]any{"units": units})

		var existing models.DataSourceInstance
		err := db.Where("type = ? AND node_id = ?", "systemd", nodeName).First(&existing).Error
		if err == nil {
			continue // already migrated
		}
		inst := models.DataSourceInstance{
			ID: ulid.Make().String(), Type: "systemd", Scope: "agent",
			NodeID: &nodeName, Name: "Systemd",
			Config: string(cfgJSON), Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(&inst).Error; err != nil {
			return fmt.Errorf("insert systemd instance for %s: %w", nodeName, err)
		}
	}

	// 2. File watcher: one instance per existing node
	redact := true
	var fwSetting models.AppSetting
	if err := db.First(&fwSetting, "key = ?", fileWatcherRedactSecretsKey).Error; err == nil {
		redact = fwSetting.Value != "false"
	}
	var nodes []models.Node
	if err := db.Find(&nodes).Error; err != nil {
		return fmt.Errorf("load nodes: %w", err)
	}
	fwCfg, _ := json.Marshal(map[string]any{"redact_secrets": redact})
	for _, node := range nodes {
		nodeName := node.Name
		var existing models.DataSourceInstance
		if db.Where("type = ? AND node_id = ?", "filewatcher", nodeName).First(&existing).Error == nil {
			continue
		}
		inst := models.DataSourceInstance{
			ID: ulid.Make().String(), Type: "filewatcher", Scope: "agent",
			NodeID: &nodeName, Name: "File Watcher",
			Config: string(fwCfg), Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(&inst).Error; err != nil {
			return fmt.Errorf("insert filewatcher instance for %s: %w", nodeName, err)
		}
	}

	// 3. Webhook instances
	for _, wType := range []string{"webhook_uptime_kuma", "webhook_watchtower"} {
		var existing models.DataSourceInstance
		if db.Where("type = ?", wType).First(&existing).Error == nil {
			continue
		}
		wCfg, _ := json.Marshal(map[string]any{"secret": envWebhookSecret})
		name := "Uptime Kuma"
		if wType == "webhook_watchtower" {
			name = "Watchtower"
		}
		inst := models.DataSourceInstance{
			ID: ulid.Make().String(), Type: wType, Scope: "server",
			Name: name, Config: string(wCfg), Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(&inst).Error; err != nil {
			return fmt.Errorf("insert %s instance: %w", wType, err)
		}
	}

	return nil
}
