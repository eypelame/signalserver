package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"signalserver/internal/config"
	"signalserver/internal/logger"
)

type ApiClient struct {
	Config *config.AppConfig
	Client *http.Client
}

func NewApiClient(cfg *config.AppConfig) *ApiClient {
	return &ApiClient{
		Config: cfg,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// UpdateCallStatus sends a status update to the PHP API.
// Status: 1 = Available, 2 = Busy
func (c *ApiClient) UpdateCallStatus(userID string, status int, userType string) {
	if c.Config.ApiWebhookUrl == "" {
		// Feature disabled if no URL configured
		return
	}

	url := fmt.Sprintf("%s/api/webrtc/update-call-status", c.Config.ApiWebhookUrl)

	payload := map[string]interface{}{
		"userId":   userID,
		"status":   status,
		"userType": userType,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Log.Errorf("[API] Error marshaling payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		logger.Log.Errorf("[API] Error creating request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Config.ApiWebhookSecret != "" {
		req.Header.Set("X-API-KEY", c.Config.ApiWebhookSecret)
	}

	// Run asynchronously to avoid blocking the signaling loop
	go func() {
		resp, err := c.Client.Do(req)
		if err != nil {
			logger.Log.Errorf("[API] Error sending status update for user %s: %v", userID, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.Log.Warnf("[API] Status update failed for user %s. HTTP Status: %d", userID, resp.StatusCode)
		}
	}()
}
