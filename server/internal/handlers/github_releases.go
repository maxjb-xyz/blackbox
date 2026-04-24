package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

const githubReleasesURL = "https://api.github.com/repos/maxjb-xyz/blackbox/releases?per_page=100"
const githubCacheTTL = time.Hour

var githubHTTPClient = &http.Client{Timeout: 10 * time.Second}

func GetGitHubReleases(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check cache
		var cachedAt models.AppSetting
		var cachedBody models.AppSetting
		atErr := database.Where("key = ?", "github_releases_cached_at").First(&cachedAt).Error
		bodyErr := database.Where("key = ?", "github_releases_body").First(&cachedBody).Error

		if atErr == nil && bodyErr == nil {
			if t, err := time.Parse(time.RFC3339, cachedAt.Value); err == nil {
				if time.Since(t) < githubCacheTTL {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					if _, err := w.Write([]byte(cachedBody.Value)); err != nil {
						log.Printf("GetGitHubReleases write cached: %v", err)
					}
					return
				}
			}
		}

		// Fetch from GitHub
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, githubReleasesURL, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build request")
			return
		}
		resp, err := githubHTTPClient.Do(req)
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to reach GitHub")
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if err != nil || resp.StatusCode != http.StatusOK {
			writeError(w, http.StatusBadGateway, "GitHub API error")
			return
		}

		// Validate JSON
		var releases []json.RawMessage
		if err := json.Unmarshal(body, &releases); err != nil {
			writeError(w, http.StatusBadGateway, "invalid GitHub response")
			return
		}

		// Cache
		now := time.Now().UTC().Format(time.RFC3339)
		if err := database.Save(&models.AppSetting{Key: "github_releases_cached_at", Value: now, UpdatedAt: time.Now()}).Error; err != nil {
			log.Printf("GetGitHubReleases cache timestamp save: %v", err)
		}
		if err := database.Save(&models.AppSetting{Key: "github_releases_body", Value: string(body), UpdatedAt: time.Now()}).Error; err != nil {
			log.Printf("GetGitHubReleases cache body save: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := w.Write(body); err != nil {
			log.Printf("GetGitHubReleases write: %v", err)
		}
	}
}
