package backend

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"signalserver/internal/config"
	"signalserver/internal/logger"
	"signalserver/internal/metrics"
)

// Connector handles server-to-server communication with the main PHP API.
type Connector struct {
	Config *config.AppConfig
	Client *http.Client
}

func NewConnector(cfg *config.AppConfig) *Connector {
	return &Connector{
		Config: cfg,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// hashMessage genera una firma HMAC-SHA256 para el cuerpo del mensaje.
func (c *Connector) hashMessage(body []byte) string {
	h := hmac.New(sha256.New, []byte(c.Config.ApiWebhookSecret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// UpdateCallStatus sends a status update to the PHP API.
// Status: 1 = Available, 2 = Busy
func (c *Connector) UpdateCallStatus(userID string, status int, userType string) {
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
		logger.Log.Errorf("[BACKEND] Error marshaling payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		logger.Log.Errorf("[BACKEND] Error creating request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Config.ApiWebhookSecret != "" {
		req.Header.Set("X-Hub-Signature-256", c.hashMessage(jsonPayload))
	}

	// Run asynchronously to avoid blocking the signaling loop
	go func() {
		start := time.Now()
		resp, err := c.Client.Do(req)
		duration := time.Since(start).Seconds()
		metrics.BackendLatencyHistogram.WithLabelValues("update-call-status").Observe(duration)

		if err != nil {
			logger.Log.Errorf("[BACKEND] Error sending status update for user %s: %v", userID, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.Log.Warnf("[BACKEND] Status update failed for user %s (Type: %s). HTTP Status: %d. URL: %s", userID, userType, resp.StatusCode, url)
		}
	}()
}

// ProcessCall sends call details to the PHP API for billing and logging.
func (c *Connector) ProcessCall(roomID, callerID, listenerID string, startTime, endTime time.Time, durationSeconds int, reason string, connType int) {
	if c.Config.ApiWebhookUrl == "" {
		return
	}

	url := fmt.Sprintf("%s/api/webrtc/process-call", c.Config.ApiWebhookUrl)

	payload := map[string]interface{}{
		"roomId":          roomID,
		"callerUserId":    callerID,
		"listenerUserId":  listenerID,
		"startTime":       startTime.Format(time.RFC3339),
		"endTime":         endTime.Format(time.RFC3339),
		"durationSeconds": durationSeconds,
		"reason":          reason,
		"connType":        connType,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Log.Errorf("[BACKEND] Error marshaling process-call payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		logger.Log.Errorf("[BACKEND] Error creating process-call request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Config.ApiWebhookSecret != "" {
		req.Header.Set("X-Hub-Signature-256", c.hashMessage(jsonPayload))
	}

	go func() {
		start := time.Now()
		resp, err := c.Client.Do(req)
		duration := time.Since(start).Seconds()
		metrics.BackendLatencyHistogram.WithLabelValues("process-call").Observe(duration)

		if err != nil {
			logger.Log.Errorf("[BACKEND] Error sending process-call for room %s: %v", roomID, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.Log.Warnf("[BACKEND] Process-call failed for room %s. HTTP Status: %d", roomID, resp.StatusCode)
		} else {
			logger.Log.Infof("[BACKEND] Call processed successfully for room %s. Duration: %ds", roomID, durationSeconds)
		}
	}()
}
