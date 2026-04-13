package notify

import (
	"context"

	"blackbox/server/internal/models"
)

func sendSlack(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
	return nil
}
