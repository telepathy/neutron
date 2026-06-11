package notify

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

type Client struct {
	Url           string
	CorpId        string
	AppId         string
	SkipTLSVerify bool
	httpClient    *http.Client
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

type messageRequest struct {
	ToSingleAccount string        `json:"to_single_account"`
	Type            string        `json:"type"`
	Message         messageBody   `json:"message"`
}

type messageBody struct {
	Id         string      `json:"id"`
	Value      string      `json:"value"`
	Url        interface{} `json:"url"`
	Avatartype int         `json:"avatartype"`
	Summary    string      `json:"summary"`
	Head       messageHead `json:"head"`
	Body       messageContent `json:"body"`
}

type messageHead struct {
	Text   string `json:"text"`
	Tcolor string `json:"tcolor"`
}

type messageContent struct {
	Content string `json:"content"`
}

func NewClient(url, corpId, appId string, skipTLS bool) *Client {
	return &Client{
		Url:           strings.TrimRight(url, "/"),
		CorpId:        corpId,
		AppId:         appId,
		SkipTLSVerify: skipTLS,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLS},
			},
		},
	}
}

func (c *Client) getAccessToken() (string, error) {
	url := fmt.Sprintf("%s/v2/gettoken?corpid=%s&appid=%s", c.Url, c.CorpId, c.AppId)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response: %s", string(body))
	}

	return tokenResp.AccessToken, nil
}

func (c *Client) SendMessage(userId, title, content string) error {
	token, err := c.getAccessToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v2/message/bot_send_to_conversation?access_token=%s", c.Url, token)
	reqBody := messageRequest{
		ToSingleAccount: userId,
		Type:            "attachment",
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

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send message failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) SendToAll(userIds []string, title, content string) {
	for _, userId := range userIds {
		if err := c.SendMessage(userId, title, content); err != nil {
			log.Printf("failed to send notify to %s: %v", userId, err)
		}
	}
}
