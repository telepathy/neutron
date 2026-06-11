package ccwork

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Webhook struct {
	Url         string
	Description string
}

type Robot struct {
	httpClient *http.Client
}

func NewRobot(skipTLS bool) *Robot {
	return &Robot{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLS},
			},
		},
	}
}

func (r *Robot) Send(webhookURL, content string) error {
	resp, err := r.httpClient.Post(webhookURL, "application/json", strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("failed to send ccwork message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ccwork send failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (r *Robot) SendToAll(webhooks []Webhook, content string) {
	for _, w := range webhooks {
		if err := r.Send(w.Url, content); err != nil {
			log.Printf("failed to send ccwork notify to %s (%s): %v", w.Description, w.Url, err)
		}
	}
}
