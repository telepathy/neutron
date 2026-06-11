package ccwork

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Webhook struct {
	Url         string
	Description string
}

type messageRequest struct {
	Type    string        `json:"type"`
	Message messageBody   `json:"message"`
}

type messageBody struct {
	Id       string      `json:"id"`
	Value    string      `json:"value"`
	Url      interface{} `json:"url"`
	Avatartype int       `json:"avatartype"`
	Summary  string      `json:"summary"`
	Head     messageHead `json:"head"`
	Body     messageContent `json:"body"`
}

type messageHead struct {
	Text   string `json:"text"`
	Tcolor string `json:"tcolor"`
}

type messageContent struct {
	Content string `json:"content"`
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

func (r *Robot) Send(webhookURL, title, content string) error {
	msg := messageRequest{
		Type: "attachment",
		Message: messageBody{
			Id:         uuid.New().String(),
			Value:      title,
			Url:        nil,
			Avatartype: 0,
			Summary:    title,
			Head: messageHead{
				Text:   title,
				Tcolor: "FC6D26",
			},
			Body: messageContent{
				Content: content,
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal ccwork message: %w", err)
	}

	resp, err := r.httpClient.Post(webhookURL, "application/json", strings.NewReader(string(data)))
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

func (r *Robot) SendToAll(webhooks []Webhook, title, content string) {
	for _, w := range webhooks {
		if err := r.Send(w.Url, title, content); err != nil {
			log.Printf("failed to send ccwork notify to %s (%s): %v", w.Description, w.Url, err)
		}
	}
}
