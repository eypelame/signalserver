package handlers

import (
	"encoding/json"

	"signalserver/internal/logger"
	"signalserver/internal/models"
)

func (h *WebSocketHandler) handleUpdateAvailability(client *models.Client, msg models.Message) {
	var payload models.UpdateAvailabilityPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		h.sendError(client, "INVALID_PAYLOAD", "Payload de disponibilidad malformado.")
		return
	}

	h.PresenceService.UpdateAvailability(client, payload.IsAvailable)
	logger.Log.Infof("[USER: %s] [CONN:%s] [IN:update-availability] - Disponibilidad: %t.", client.LogKey(), client.ConnectionID, payload.IsAvailable)
	h.broadcastClientList()
}

func (h *WebSocketHandler) handleUpdatePushToken(client *models.Client, msg models.Message) {
	var payload models.UpdatePushTokenPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		h.sendError(client, "INVALID_PAYLOAD", "Payload de push token malformado.")
		return
	}

	h.PresenceService.UpdatePushToken(client.UserID, client.UserRole, payload.PushToken)
	if payload.PushToken == "" {
		logger.Log.Infof("[USER: %s] [CONN:%s] [IN:update-push-token] - Push token eliminado.", client.LogKey(), client.ConnectionID)
	} else {
		logger.Log.Infof("[USER: %s] [CONN:%s] [IN:update-push-token] - Push token actualizado.", client.LogKey(), client.ConnectionID)
	}
}

func (h *WebSocketHandler) handleClearFcmAndDisconnect(client *models.Client, msg models.Message) {
	h.PresenceService.UpdatePushToken(client.UserID, client.UserRole, "")
	h.PresenceService.SetUserAvailability(client.UserID, client.UserRole, false)

	// Al ser un logout explícito, marcamos como No disponible (4) en la base de datos
	go h.Backend.UpdateCallStatus(client.UserID, 4, client.UserRole)

	logger.Log.Infof("[USER: %s] [CONN:%s] [IN:clear-fcm-and-disconnect] - FCM Token eliminado y desconectando.", client.LogKey(), client.ConnectionID)
	h.unregister <- client
}
