package models

import "encoding/json"

// Message representa la estructura base de los mensajes WebSocket intercambiados.
type Message struct {
	Type   string          `json:"type"`
	From   string          `json:"from,omitempty"` // ConnectionID del remitente
	To     string          `json:"to,omitempty"`   // UserID o ConnectionID del destinatario
	RoomID string          `json:"roomId,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// CallDTO es el objeto de transferencia para inicio y respuesta de llamadas.
type CallDTO struct {
	SDP             string `json:"sdp,omitempty"`
	SDPType         string `json:"sdpType,omitempty"`
	Reason          string `json:"reason,omitempty"`
	CallerUserID    string `json:"callerUserId,omitempty"`
	CallerUserName  string `json:"callerUserName,omitempty"`
	RecipientUserID string `json:"recipientUserId,omitempty"`
}

// Validate verifica que el DTO sea válido para el tipo de mensaje.
func (c *CallDTO) Validate(msgType string) bool {
	switch msgType {
	case "call-request":
		return c.SDP != "" && c.SDPType != ""
	case "call-accept":
		return c.SDP != "" && c.SDPType != ""
	case "call-reject":
		return true // Reason es opcional
	default:
		return false
	}
}

// SignalingDTO es el payload para intercambio de candidatos ICE.
type SignalingDTO struct {
	Candidate interface{} `json:"candidate"` // Objeto ICECandidate
}

// UpdateAvailabilityPayload es el payload para actualizar la disponibilidad.
type UpdateAvailabilityPayload struct {
	IsAvailable bool `json:"isAvailable"`
}

// UpdatePushTokenPayload es el payload para actualizar el token push.
type UpdatePushTokenPayload struct {
	PushToken string `json:"pushToken"`
}

// ClientsListPayload es el payload para enviar la lista de clientes.
type ClientsListPayload struct {
	TotalAvailable int          `json:"totalAvailable"`
	Clients        []ClientInfo `json:"clients"`
}

// ClientInfo es una versión simplificada de Client para la lista de clientes.
type ClientInfo struct {
	UserID       string `json:"userId"`
	UserName     string `json:"userName"`
	ConnectionID string `json:"connectionId"`
	IsAvailable  bool   `json:"isAvailable"`
	ClientType   string `json:"clientType"` // "web" o "mobile"
	UserRole     string `json:"userRole"`   // "cliente" o "escucha"
}

// ErrorPayload es el payload para mensajes de error genéricos.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ConnectionReportDTO es el payload para informar sobre el tipo de red WebRTC.
type ConnectionReportDTO struct {
	CandidateType string `json:"candidateType"` // "host", "srflx" o "relay"
}
