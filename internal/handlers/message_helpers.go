package handlers

import (
	"encoding/json"
	"time"

	"signalserver/internal/logger"
	"signalserver/internal/metrics"
	"signalserver/internal/models"
)

func (h *WebSocketHandler) sendError(client *models.Client, code, message string) {
	data, _ := json.Marshal(models.ErrorPayload{Code: code, Message: message})
	errorMsg := models.Message{
		Type: "error",
		To:   client.ConnectionID,
		Data: data,
	}
	jsonErrorMsg, _ := json.Marshal(errorMsg)

	metrics.ErrorsTotal.Inc()
	select {
	case client.Send <- jsonErrorMsg:
	case <-time.After(h.Config.WriteTimeout):
		logger.Log.Warnf("Tiempo de espera agotado al enviar error a %s", client.ConnectionID)
	}
}

func (h *WebSocketHandler) sendConnectionAck(client *models.Client) {
	data, _ := json.Marshal(map[string]string{"clientId": client.ConnectionID, "userId": client.UserID})
	ackMsg := models.Message{
		Type: "connection-ack",
		To:   client.ConnectionID,
		Data: data,
	}

	h.sendMessageToClient(client, ackMsg)
	logger.Log.Infof("[USER: %s] [CONN:%s] [OUT:connection-ack] - Enviado ACK de conexión.", client.LogKey(), client.ConnectionID)
}

func (h *WebSocketHandler) sendCallRequestAck(client *models.Client, status, reason, roomID string) {
	data, _ := json.Marshal(map[string]string{"status": status, "reason": reason})
	ackMsg := models.Message{
		Type:   "call-request-ack",
		To:     client.ConnectionID,
		RoomID: roomID,
		Data:   data,
	}

	h.sendMessageToClient(client, ackMsg)
	logger.Log.Infof("[USER: %s] [CONN:%s] [OUT:call-request-ack] - Status: %s, Reason: %s [RoomID: %s].", client.LogKey(), client.ConnectionID, status, reason, roomID)
}

func (h *WebSocketHandler) sendCallRinging(client *models.Client, roomID, targetUserID string) {
	data, _ := json.Marshal(map[string]string{"targetUserId": targetUserID})
	ringingMsg := models.Message{
		Type:   "call-ringing",
		To:     client.ConnectionID,
		RoomID: roomID,
		Data:   data,
	}

	h.sendMessageToClient(client, ringingMsg)
	logger.Log.Infof("[USER: %s] [CONN:%s] [OUT:call-ringing] - Informando que [USER %s] está sonando [RoomID: %s].", client.LogKey(), client.ConnectionID, targetUserID, roomID)
}

func (h *WebSocketHandler) sendSessionReplacedMessage(client *models.Client, message string) {
	data, _ := json.Marshal(models.ErrorPayload{Code: "SESSION_REPLACED", Message: message})
	msg := models.Message{
		Type: "session-replaced",
		To:   client.ConnectionID,
		Data: data,
	}

	h.sendMessageToClient(client, msg)
}

func (h *WebSocketHandler) sendMessageToClient(client *models.Client, msg models.Message) {
	jsonMessage, err := json.Marshal(msg)
	if err != nil {
		logger.Log.Errorf("Error al serializar mensaje para %s: %v", client.ConnectionID, err)
		return
	}
	select {
	case client.Send <- jsonMessage:
		metrics.MessagesSentTotal.Inc()
	case <-time.After(h.Config.WriteTimeout):
		logger.Log.Warnf("[USER: %s] [CONN:%s] [OUT:%s] - Timeout al enviar mensaje.", client.LogKey(), client.ConnectionID, msg.Type)
	}
}

func (h *WebSocketHandler) sendHangupMessage(client *models.Client, roomID, reasonCode, reasonMessage string) {
	data, _ := json.Marshal(map[string]string{"reasonCode": reasonCode, "reasonMessage": reasonMessage})
	hangupMsg := models.Message{
		Type:   "hangup",
		To:     client.ConnectionID,
		RoomID: roomID,
		Data:   data,
	}

	h.sendMessageToClient(client, hangupMsg)
}

func (h *WebSocketHandler) notifyCallStatus(room *models.Room, status int) {
	go h.Backend.UpdateCallStatus(room.CallerUserID, status, room.CallerRole)
	go h.Backend.UpdateCallStatus(room.ListenerUserID, status, room.ListenerRole)
}

func (h *WebSocketHandler) finalizeRoomBilling(room *models.Room, reason string) {
	endTime := time.Now()
	duration := int(endTime.Sub(room.CallStartTime).Seconds())
	go h.Backend.ProcessCall(room.ID, room.CallerUserID, room.ListenerUserID, room.CallStartTime, endTime, duration, reason, room.ConnType)
}
