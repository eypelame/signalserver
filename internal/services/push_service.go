package services

import (
	"context"
	"fmt"
	"time"

	"signalserver/internal/fcm"
	"signalserver/internal/logger"
	"signalserver/internal/metrics"
	"signalserver/internal/models"
)

// PushService gestiona el envío de notificaciones push ricas en contexto de negocio.
type PushService struct {
	fcmClient fcm.FCMClient
}

// NewPushService crea una nueva instancia de PushService.
func NewPushService(fcmClient fcm.FCMClient) *PushService {
	return &PushService{
		fcmClient: fcmClient,
	}
}

// SendCallRequest envía una notificación de llamada entrante.
func (s *PushService) SendCallRequest(ctx context.Context, callerClient *models.Client, targetUserID, roomID, sdp, sdpType, pushToken string) error {
	if s.fcmClient == nil {
		return fmt.Errorf("FCM client not initialized")
	}

	notificationData := map[string]string{
		"type":           "call-request",
		"callerUserId":   callerClient.UserID,
		"callerUserName": callerClient.UserName,
		"roomId":         roomID,
		"sdp":            sdp,
		"sdpType":        sdpType,
	}

	title := fmt.Sprintf("Llamada entrante de %s", callerClient.UserName)
	body := "Estás recibiendo una llamada."

	// Enviar de forma asíncrona pero permitiendo manejo de error básico si se desea esperar
	go func() {
		requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.fcmClient.SendPushNotification(requestCtx, pushToken, title, body, notificationData); err != nil {
			logger.Log.Errorf("[PUSH] [ACTION:call-request] [USER: %s] - Error: %v", targetUserID, err)
			metrics.FCMNotificationsStatusTotal.WithLabelValues("error").Inc()
		} else {
			logger.Log.Infof("[PUSH] [ACTION:call-request] [USER: %s] - Éxito [RoomID: %s]", targetUserID, roomID)
			metrics.FCMNotificationsStatusTotal.WithLabelValues("success").Inc()
		}
	}()

	return nil
}

// SendHangup envía una notificación de finalización de llamada.
func (s *PushService) SendHangup(ctx context.Context, targetUserID, roomID, reason, pushToken string) error {
	if s.fcmClient == nil {
		return fmt.Errorf("FCM client not initialized")
	}

	notificationData := map[string]string{
		"type":   "hangup",
		"roomId": roomID,
		"reason": reason,
	}

	go func() {
		requestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.fcmClient.SendPushNotification(requestCtx, pushToken, "Llamada finalizada", reason, notificationData); err != nil {
			logger.Log.Errorf("[PUSH] [ACTION:hangup] [USER: %s] - Error: %v", targetUserID, err)
		} else {
			logger.Log.Infof("[PUSH] [ACTION:hangup] [USER: %s] - Éxito [RoomID: %s]", targetUserID, roomID)
		}
	}()

	return nil
}
