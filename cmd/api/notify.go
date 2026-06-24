package main

import (
	"neutron/internal/ccwork"
)

// sendNotifications fans a title/content message out to both notification
// channels configured for a project: per-user IM messages and CCWork group
// webhooks. Each send runs in its own goroutine, matching the original
// fire-and-forget semantics.
func (s *Server) sendNotifications(projectId, title, content string) {
	if s.notifyClient != nil {
		if recipients, err := s.repo.ListNotifyRecipients(projectId); err == nil {
			for _, r := range recipients {
				go s.notifyClient.SendMessage(r.UserId, title, content)
			}
		}
	}
	if webhooks, err := s.repo.ListCCWebhooks(projectId); err == nil && len(webhooks) > 0 {
		ccWebhooks := make([]ccwork.Webhook, len(webhooks))
		for i, w := range webhooks {
			ccWebhooks[i] = ccwork.Webhook{Url: w.WebhookUrl, Description: w.Description}
		}
		go s.ccworkRobot.SendToAll(ccWebhooks, title, content)
	}
}
