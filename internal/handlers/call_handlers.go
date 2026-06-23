// handlers/call_handlers.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"signalserver/internal/logger"
	"signalserver/internal/models"
)

func (h *WebSocketHandler) handleCallRequest(client *models.Client, msg models.Message) {
	var payload models.CallDTO
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		h.sendError(client, "INVALID_PAYLOAD", "Error al procesar el payload de la llamada.")
		return
	}

	if !payload.Validate(msg.Type) {
		h.sendError(client, "INVALID_PAYLOAD", "El payload debe contener 'sdp' y 'sdpType'.")
		return
	}

	targetUserID := msg.To
	roomID, err := h.CallService.InitiateCall(client, targetUserID)
	if err != nil {
		logger.Log.Warnf("[USER: %s] [CONN:%s] [IN:call-request] - Fallido: %v.", client.LogKey(), client.ConnectionID, err)
		h.sendCallRequestAck(client, "failed", err.Error(), "")
		return
	}

	room, _ := h.RoomManager.GetRoom(roomID)
	h.sendCallRinging(client, roomID, targetUserID)
	h.sendCallRequestAck(client, "ringing", "", roomID)
	// Notificar a las conexiones activas del target
	targetConnections := h.PresenceService.GetUserConnections(targetUserID, "escucha")
	if len(targetConnections) > 0 {
		payload.CallerUserName = client.UserName
		payload.CallerUserID = client.UserID

		// Aplicar optimización de audio si está activa
		if h.Config.EnableWebRTCAudioOptimization {
			payload.SDP = h.optimizeSDP(payload.SDP)
			logger.Log.Infof("[SDP-OPT] [USER: %s] [IN:call-request] - SDP Oferta optimizado a %d bps.", client.UserID, h.Config.WebRTCAudioBitrate)
		}

		newData, _ := json.Marshal(payload)

		notificationMsg := models.Message{
			Type:   "call-request",
			From:   client.ConnectionID,
			RoomID: roomID,
			Data:   newData,
		}

		for _, targetConn := range targetConnections {
			notificationMsg.To = targetConn.ConnectionID
			h.sendMessageToClient(targetConn, notificationMsg)
		}

		go h.startCallRequestTimer(room, client)
		h.broadcastClientList()
	} else {
		// Notificar vía Push si no hay conexiones activas
		if pushToken, ok := h.PresenceService.GetPushToken(targetUserID, "escucha"); ok {
			logger.Log.Infof("[USER: %s] [PUSH] - Notificando llamada vía Push a %s.", client.LogKey(), targetUserID)

			// Aplicar optimización de audio si está activa antes de enviar via Push
			sdpToSend := payload.SDP
			if h.Config.EnableWebRTCAudioOptimization {
				sdpToSend = h.optimizeSDP(sdpToSend)
				logger.Log.Infof("[SDP-OPT] [USER: %s] [IN:call-request] - SDP Oferta (Push) optimizado.", client.UserID)
			}

			go h.PushService.SendCallRequest(context.Background(), client, targetUserID, roomID, sdpToSend, payload.SDPType, pushToken)
			go h.startCallRequestTimer(room, client)
			h.broadcastClientList()
		} else {
			// Si no hay forma de notificar, revertir inicio de llamada
			logger.Log.Warnf("[USER: %s] [CONN:%s] [IN:call-request] - Fallido: usuario %s sin conexión ni Push Token.", client.LogKey(), client.ConnectionID, targetUserID)
			h.sendCallRequestAck(client, "failed", "El usuario no está disponible.", roomID)
			h.CallService.RejectCall(roomID, "no_notification_channel")
			h.broadcastClientList()
		}
	}
}

func (h *WebSocketHandler) handleCallAccept(client *models.Client, msg models.Message) {
	var payload models.CallDTO
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		h.sendError(client, "INVALID_PAYLOAD", "Error al procesar el payload de aceptación.")
		return
	}

	if !payload.Validate(msg.Type) {
		h.sendError(client, "INVALID_PAYLOAD", "El payload debe contener 'sdp' y 'sdpType'.")
		return
	}

	room, err := h.CallService.AcceptCall(client, msg.RoomID)
	if err != nil {
		h.sendError(client, "CALL_ACCEPT_FAILED", err.Error())
		return
	}

	logger.Log.Infof("[USER: %s] [CONN:%s] [IN:call-accept] - Llamada en [RoomID: %s] iniciada.", client.LogKey(), client.ConnectionID, msg.RoomID)
	go h.startCallTimer(room)

	// Notificar al llamante
	callerConnections := h.PresenceService.GetUserConnections(room.CallerUserID, room.CallerRole)
	for _, callerConn := range callerConnections {
		payload.RecipientUserID = client.UserID

		// Aplicar optimización de audio si está activa
		if h.Config.EnableWebRTCAudioOptimization {
			payload.SDP = h.optimizeSDP(payload.SDP)
			logger.Log.Infof("[SDP-OPT] [USER: %s] [IN:call-accept] - SDP Respuesta optimizado a %d bps.", client.UserID, h.Config.WebRTCAudioBitrate)
		}

		newData, _ := json.Marshal(payload)
		responseMsg := models.Message{
			Type:   "call-accept",
			From:   client.ConnectionID,
			To:     callerConn.ConnectionID,
			RoomID: msg.RoomID,
			Data:   newData,
		}
		h.sendMessageToClient(callerConn, responseMsg)
	}

	// Entregar mensajes acumulados (si los hay) para el escucha
	for _, bufferedMsg := range room.FlushBuffer(client.UserID) {
		bufferedMsg.To = client.ConnectionID
		h.sendMessageToClient(client, bufferedMsg)
	}
}

func (h *WebSocketHandler) handleCallReject(client *models.Client, msg models.Message) {
	var payload models.CallDTO
	json.Unmarshal(msg.Data, &payload) // Ignoramos error, usamos valor por defecto si falla

	room, err := h.CallService.RejectCall(msg.RoomID, payload.Reason)
	if err != nil {
		h.sendError(client, "CALL_REJECT_FAILED", err.Error())
		return
	}

	// Notificar al llamante
	callerConnections := h.PresenceService.GetUserConnections(room.CallerUserID, room.CallerRole)
	for _, callerConn := range callerConnections {
		payload.RecipientUserID = client.UserID
		newData, _ := json.Marshal(payload)
		responseMsg := models.Message{
			Type:   "call-rejected",
			From:   client.ConnectionID,
			To:     callerConn.ConnectionID,
			RoomID: msg.RoomID,
			Data:   newData,
		}
		h.sendMessageToClient(callerConn, responseMsg)
	}
	h.broadcastClientList()
}

func (h *WebSocketHandler) handleHangup(client *models.Client, msg models.Message) {
	room, err := h.CallService.Hangup(client.UserID, client.UserRole, msg.RoomID, "hangup")
	if err != nil {
		logger.Log.Warnf("[USER: %s] [CONN:%s] [IN:hangup] - Ignorado: %v.", client.LogKey(), client.ConnectionID, err)
		return
	}

	logger.Log.Infof("[USER: %s] [CONN:%s] [IN:hangup] - Finalizando [RoomID: %s].", client.LogKey(), client.ConnectionID, msg.RoomID)

	// Notificar a todos los involucrados (menos al actor)
	targets := []struct {
		ID   string
		Role string
	}{
		{ID: room.CallerUserID, Role: room.CallerRole},
		{ID: room.ListenerUserID, Role: room.ListenerRole},
	}

	for _, target := range targets {
		connections := h.PresenceService.GetUserConnections(target.ID, target.Role)
		for _, conn := range connections {
			if conn.ConnectionID != client.ConnectionID {
				h.sendHangupMessage(conn, msg.RoomID, "user_hung_up", fmt.Sprintf("El usuario %s ha colgado.", client.UserID))
			}
		}

		if len(connections) == 0 {
			if pushToken, ok := h.PresenceService.GetPushToken(target.ID, target.Role); ok {
				h.PushService.SendHangup(context.Background(), target.Role+"_"+target.ID, msg.RoomID, fmt.Sprintf("El usuario %s ha colgado.", client.UserID), pushToken)
			}
		}
	}
	h.broadcastClientList()
}

func (h *WebSocketHandler) handleSignalingMessage(client *models.Client, msg models.Message) {
	room, ok := h.RoomManager.GetRoom(msg.RoomID)
	if !ok {
		return
	}

	status := room.GetStatus()
	if status != "pending" && status != "active" {
		return
	}

	var targetUserID string
	var targetRole string
	if client.UserID == room.CallerUserID {
		targetUserID = room.ListenerUserID
		targetRole = room.ListenerRole
	} else {
		targetUserID = room.CallerUserID
		targetRole = room.CallerRole
	}

	targetConnections := h.PresenceService.GetUserConnections(targetUserID, targetRole)
	if len(targetConnections) == 0 {
		// Bufferizar mensaje si el destinatario no tiene conexiones activas
		if targetUserID == room.CallerUserID || targetUserID == room.ListenerUserID {
			// Determinar el rol del destinatario para el log
			targetRole := "escucha"
			if targetUserID == room.CallerUserID {
				targetRole = room.CallerRole
			}
			recipientKey := targetRole + "_" + targetUserID

			logger.Log.Debugf("[SIGNAL-BUFFER] [RoomID: %s] - Almacenando mensaje tipo %s para usuario %s", msg.RoomID, msg.Type, recipientKey)
			room.BufferMessage(targetUserID, msg)
		}
		return
	}

	for _, targetConn := range targetConnections {
		msg.From = client.ConnectionID
		msg.To = targetConn.ConnectionID

		// Aplicar optimización si es una oferta o respuesta SDP y está habilitada
		if h.Config.EnableWebRTCAudioOptimization && (msg.Type == "sdp-offer" || msg.Type == "sdp-answer") {
			var payload models.CallDTO
			if err := json.Unmarshal(msg.Data, &payload); err == nil {
				payload.SDP = h.optimizeSDP(payload.SDP)
				msg.Data, _ = json.Marshal(payload)
				logger.Log.Debugf("[SDP-OPT] [SIGNAL:%s] - SDP optimizado automáticamente.", msg.Type)
			}
		}

		h.sendMessageToClient(targetConn, msg)
	}
}

func (h *WebSocketHandler) startCallTimer(room *models.Room) {
	timer := time.NewTimer(room.CallMaxDuration)
	defer timer.Stop()

	select {
	case <-timer.C:
		if _, err := h.CallService.Hangup(room.CallerUserID, room.CallerRole, room.ID, "timeout"); err == nil {
			h.notifyCallTimeout(room)
			h.broadcastClientList()
		}
	case <-room.TimerCtx.Done():
		// Llamada finalizada normalmente, salir limpiamente
		logger.Log.Debugf("[TIMER] Timer cancelado para sala %s.", room.ID)
		return
	}
}

func (h *WebSocketHandler) notifyCallTimeout(room *models.Room) {
	targets := []struct {
		ID   string
		Role string
	}{
		{ID: room.CallerUserID, Role: room.CallerRole},
		{ID: room.ListenerUserID, Role: room.ListenerRole},
	}

	for _, target := range targets {
		connections := h.PresenceService.GetUserConnections(target.ID, target.Role)
		for _, conn := range connections {
			h.sendHangupMessage(conn, room.ID, "timeout", "La duración máxima de la llamada ha sido alcanzada.")
		}
	}
}

func (h *WebSocketHandler) startCallRequestTimer(room *models.Room, callerClient *models.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	room.CallRequestCancelFunc = cancel

	timeout := time.Duration(h.Config.CallRequestTimeoutSeconds) * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		if _, err := h.CallService.RejectCall(room.ID, "timeout"); err == nil {
			callerConnections := h.PresenceService.GetUserConnections(room.CallerUserID, room.CallerRole)
			for _, conn := range callerConnections {
				h.sendCallRequestAck(conn, "failed", "timeout", room.ID)
			}
			h.broadcastClientList()
		}
	case <-ctx.Done():
		// Solicitud de llamada respondida antes del timeout
		return
	}
}

func (h *WebSocketHandler) handleConnectionReport(client *models.Client, msg models.Message) {
	var payload models.ConnectionReportDTO
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		logger.Log.Warnf("[USER: %s] [CONN:%s] [IN:connection-report] - Error decodificando payload: %v", client.LogKey(), client.ConnectionID, err)
		return
	}

	connTypeVerbose := payload.CandidateType
	connTypeInt := 0
	switch payload.CandidateType {
	case "host":
		connTypeVerbose = "Host (Direct/Peer-to-Peer)"
		connTypeInt = 1
	case "srflx":
		connTypeVerbose = "Srflx (STUN)"
		connTypeInt = 2
	case "relay":
		connTypeVerbose = "Relay (TURN)"
		connTypeInt = 3
	}

	// Guardar el tipo de conexión en la sala
	if room, ok := h.RoomManager.GetRoom(msg.RoomID); ok {
		room.SetConnType(connTypeInt)
	}

	logger.Log.Infof("[ICE-REPORT] [USER: %s] [RoomID: %s] - [CONN_TYPE : %s]", client.LogKey(), msg.RoomID, connTypeVerbose)
	if h.TrafficLogger != nil {
		h.TrafficLogger.LogRawString(fmt.Sprintf("[ICE-REPORT] [USER: %s] [RoomID: %s] - [CONN_TYPE : %s]", client.LogKey(), msg.RoomID, connTypeVerbose))
	}
}

// optimizeSDP busca la línea de configuración de OPUS y ajusta el bitrate y parámetros de calidad.
func (h *WebSocketHandler) optimizeSDP(sdp string) string {
	// Localizar la línea a=fmtp:111 (payload opus habitual)
	// Formato esperado: a=fmtp:111 minptime=10;useinbandfec=1
	re := regexp.MustCompile(`(a=fmtp:111 .*)\r?\n`)
	match := re.FindStringSubmatch(sdp)

	if len(match) > 1 {
		fmtpLine := strings.TrimSpace(match[1])
		// Evitar duplicados si ya tiene maxaveragebitrate
		if !strings.Contains(fmtpLine, "maxaveragebitrate") {
			// Inyectar parámetros de alta calidad
			newFmtpLine := fmt.Sprintf("%s;maxaveragebitrate=%d;stereo=1;useinbandfec=1", fmtpLine, h.Config.WebRTCAudioBitrate)
			return strings.Replace(sdp, match[1], newFmtpLine, 1)
		}
	}

	return sdp
}
