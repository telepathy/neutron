package main

import (
	"neutron/internal/ccwork"
	"neutron/internal/model"
)

// sendJobNotifications fans a title/content message out to a job's configured
// notification targets: IM personal messages for each user, and CCWork group
// robot webhooks for each group URL. Each send runs in its own goroutine,
// preserving fire-and-forget semantics. A nil or empty Notify sends nothing.
func (s *Server) sendJobNotifications(n *model.Notify, title, content string) {
	if n == nil {
		return
	}
	if s.notifyClient != nil {
		for _, u := range n.Users {
			go s.notifyClient.SendMessage(u, title, content)
		}
	}
	if len(n.Groups) > 0 {
		ccWebhooks := make([]ccwork.Webhook, len(n.Groups))
		for i, url := range n.Groups {
			ccWebhooks[i] = ccwork.Webhook{Url: url}
		}
		go s.ccworkRobot.SendToAll(ccWebhooks, title, content)
	}
}
