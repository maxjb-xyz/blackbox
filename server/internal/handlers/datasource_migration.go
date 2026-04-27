package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const dataSourcesMigratedKey = "data_sources_migrated"

// MigrateDataSources seeds data_source_instances from legacy tables.
// Safe to call on every startup — skips rows that already exist.
func MigrateDataSources(db *gorm.DB, envWebhookSecret string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var migrationSetting models.AppSetting
		if err := tx.First(&migrationSetting, "key = ?", dataSourcesMigratedKey).Error; err == nil {
			if strings.EqualFold(migrationSetting.Value, "true") {
				return nil
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load migration marker: %w", err)
		}

		// 1. Systemd: one instance per node in systemd_unit_configs
		var systemdConfigs []models.SystemdUnitConfig
		if err := tx.Find(&systemdConfigs).Error; err != nil {
			return fmt.Errorf("load systemd configs: %w", err)
		}
		for _, cfg := range systemdConfigs {
			nodeName := cfg.NodeName
			var units []string
			if err := json.Unmarshal([]byte(cfg.Units), &units); err != nil {
				log.Printf("MigrateDataSources: invalid systemd units for node %s: %v (raw=%q)", nodeName, err, cfg.Units)
				units = []string{}
			}
			cfgJSON, err := json.Marshal(map[string]any{"units": units})
			if err != nil {
				log.Printf("MigrateDataSources: failed to marshal systemd config for node %s: %v", nodeName, err)
				return fmt.Errorf("marshal systemd config for %s: %w", nodeName, err)
			}

			var existing models.DataSourceInstance
			err = tx.Where("type = ? AND node_id = ?", "systemd", nodeName).First(&existing).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("check systemd instance for %s: %w", nodeName, err)
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				inst := models.DataSourceInstance{
					ID: ulid.Make().String(), Type: "systemd", Scope: "agent",
					NodeID: &nodeName, Name: "Systemd",
					Config: string(cfgJSON), Enabled: true,
					CreatedAt: now, UpdatedAt: now,
				}
				if err := tx.Create(&inst).Error; err != nil {
					return fmt.Errorf("insert systemd instance for %s: %w", nodeName, err)
				}
			}
			if err := tx.Delete(&models.SystemdUnitConfig{}, "node_name = ?", nodeName).Error; err != nil {
				return fmt.Errorf("delete legacy systemd config for %s: %w", nodeName, err)
			}
		}

		// 2. File watcher: one instance per capable node
		redact := true
		var fwSetting models.AppSetting
		if err := tx.First(&fwSetting, "key = ?", fileWatcherRedactSecretsKey).Error; err == nil {
			redact = !strings.EqualFold(fwSetting.Value, "false")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load file watcher setting: %w", err)
		}
		var nodes []models.Node
		if err := tx.Find(&nodes).Error; err != nil {
			return fmt.Errorf("load nodes: %w", err)
		}
		fwCfg, err := json.Marshal(map[string]any{"redact_secrets": redact})
		if err != nil {
			return fmt.Errorf("marshal filewatcher config: %w", err)
		}
		for _, node := range nodes {
			nodeName := node.Name
			if !nodeHasCapability(node.Capabilities, "filewatcher") {
				// Treat empty capability payloads as legacy nodes that still need filewatcher seeding.
				if strings.TrimSpace(node.Capabilities) == "" || node.Capabilities == "[]" {
					capsJSON, err := json.Marshal([]string{"filewatcher"})
					if err != nil {
						return fmt.Errorf("marshal capabilities for %s: %w", nodeName, err)
					}
					log.Printf("MigrateDataSources: seeding legacy capabilities for node %s from %q to %s", nodeName, node.Capabilities, string(capsJSON))
					if err := tx.Model(&models.Node{}).Where("name = ?", nodeName).Update("capabilities", string(capsJSON)).Error; err != nil {
						log.Printf("MigrateDataSources: failed to update capabilities for node %s: %v", nodeName, err)
						return fmt.Errorf("update capabilities for %s: %w", nodeName, err)
					}
				} else {
					continue
				}
			}
			var existing models.DataSourceInstance
			err := tx.Where("type = ? AND node_id = ?", "filewatcher", nodeName).First(&existing).Error
			if err == nil {
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("check filewatcher instance for %s: %w", nodeName, err)
			}
			inst := models.DataSourceInstance{
				ID: ulid.Make().String(), Type: "filewatcher", Scope: "agent",
				NodeID: &nodeName, Name: "File Watcher",
				Config: string(fwCfg), Enabled: true,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&inst).Error; err != nil {
				return fmt.Errorf("insert filewatcher instance for %s: %w", nodeName, err)
			}
		}

		// 3. Webhook instances
		for _, wType := range []string{"webhook_uptime_kuma", "webhook_watchtower"} {
			var existing models.DataSourceInstance
			err := tx.Where("type = ? AND scope = ?", wType, models.ScopeServer).First(&existing).Error
			if err == nil {
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("check %s instance: %w", wType, err)
			}
			wCfg, err := json.Marshal(map[string]any{"secret": envWebhookSecret})
			if err != nil {
				log.Printf("MigrateDataSources: failed to marshal webhook config for %s: %v", wType, err)
				return fmt.Errorf("marshal webhook config for %s: %w", wType, err)
			}
			name := "Uptime Kuma"
			if wType == "webhook_watchtower" {
				name = "Watchtower"
			}
			inst := models.DataSourceInstance{
				ID: ulid.Make().String(), Type: wType, Scope: "server",
				Name: name, Config: string(wCfg), Enabled: true,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&inst).Error; err != nil {
				return fmt.Errorf("insert %s instance: %w", wType, err)
			}
		}

		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "key"}},
			DoUpdates: clause.Assignments(map[string]any{
				"value":      "true",
				"updated_at": now,
			}),
		}).Create(&models.AppSetting{
			Key:       dataSourcesMigratedKey,
			Value:     "true",
			UpdatedAt: now,
		}).Error
	})
}

func nodeHasCapability(rawCapabilities, capability string) bool {
	var capabilities []string
	if err := json.Unmarshal([]byte(rawCapabilities), &capabilities); err != nil {
		return false
	}
	return slices.Contains(capabilities, capability)
}
