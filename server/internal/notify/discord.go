package notify

import (
	"context"

	"blackbox/server/internal/models"
)

func sendDiscord(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
	return nil
}
