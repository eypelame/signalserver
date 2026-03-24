package handlers

import (
	"net/http"
	"time"

	"signalserver/internal/logger"
)

func (h *WebSocketHandler) startRoomCleanupWorker() {
	interval := time.Duration(h.Config.CleanupIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		h.startRoomCleanup()
	}
}

func (h *WebSocketHandler) startRoomCleanup() {
	rooms := h.RoomManager.GetAllRoomsSnapshot()
	now := time.Now()
	for roomID, room := range rooms {
		if room.IsTimedOut(now) {
			logger.Log.Infof("[CLEANUP] Sala %s expirada. Eliminando.", roomID)

			// Usamos el servicio para un cierre limpio y consistente
			h.CallService.Hangup(room.CallerUserID, room.CallerRole, roomID, "timeout")

			// Notificar a los clientes si aún están conectados
			h.notifyCallTimeout(room)
		}
	}
	h.broadcastClientList()
}

func HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
