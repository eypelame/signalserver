package fcm

import (
	"context"
	"fmt"

	"signalserver/internal/config"
	"signalserver/internal/logger"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// FCMClient define la interfaz para el cliente FCM.
type FCMClient interface {
	SendPushNotification(ctx context.Context, pushToken, title, body string, data map[string]string) error
}

// firebaseClient implementa FCMClient usando el SDK de Firebase Admin.
type firebaseClient struct {
	msgClient *messaging.Client
}

// NewFCMClient inicializa y retorna una nueva instancia de FCMClient.
func NewFCMClient(ctx context.Context, cfg *config.AppConfig) (FCMClient, error) {
	if cfg.FCMServiceAccountKeyPath == "" {
		logger.Log.Warn("[FCM] - FCM_SERVICE_ACCOUNT_KEY_PATH no configurado. Las notificaciones push no funcionarán.")
		return nil, nil // Retornar nil, nil si FCM no está configurado
	}

	opt := option.WithCredentialsFile(cfg.FCMServiceAccountKeyPath)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("error al inicializar la aplicación de Firebase: %w", err)
	}

	msgClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("error al obtener el cliente de mensajería de Firebase: %w", err)
	}

	// El log de éxito se maneja centralizadamente en el Validator
	return &firebaseClient{msgClient: msgClient}, nil
}

// SendPushNotification envía una notificación push a un dispositivo específico.
func (f *firebaseClient) SendPushNotification(ctx context.Context, pushToken, title, body string, data map[string]string) error {
	if f.msgClient == nil {
		logger.Log.Warn("[FCM] - Intento de enviar notificación push, pero el cliente FCM no está inicializado.")
		return fmt.Errorf("cliente FCM no inicializado")
	}

	// Asegurarse de que data no sea nulo
	if data == nil {
		data = make(map[string]string)
	}
	// Añadir título y cuerpo al mapa de datos
	data["title"] = title
	data["body"] = body

	message := &messaging.Message{
		Data:  data,
		Token: pushToken,
		// Configuración específica para Android (FCM) y iOS (APNs) puede ir aquí
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					ContentAvailable: true,
				},
			},
		},
	}

	response, err := f.msgClient.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("error al enviar notificación FCM: %w", err)
	}

	logger.Log.Infof("[FCM] - Notificación enviada a %s: %s", pushToken, response)
	return nil
}
