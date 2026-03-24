package handlers

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"signalserver/internal/auth"
	"signalserver/internal/config"
	"signalserver/internal/logger"
)

type ICECredential struct {
	URLs       interface{} `json:"urls"` // Can be string or []string
	Username   string      `json:"username,omitempty"`
	Credential string      `json:"credential,omitempty"`
}

type ICEServersResponse struct {
	IceServers []ICECredential `json:"iceServers"`
}

// GenerateTurnCredentials crea las credenciales temporales TURN REST API usando HMAC-SHA1.
func HandleTurnCredentials(cfg *config.AppConfig, jwtValidator auth.JWTValidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CORS headers (handled by middleware usually, but good to be explicit for API)
		w.Header().Set("Content-Type", "application/json")

		// Extraer el token del header Authorization
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logger.Log.Warn("[TURN API] Solicitud sin header de Autorización")
			http.Error(w, "No Authorization header provided", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			logger.Log.Warn("[TURN API] Formato de Autorización incorrecto")
			http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		// Validar el JWT
		claims, err := jwtValidator.ValidateToken(tokenString)
		if err != nil {
			logger.Log.Warnf("[TURN API] Token inválido: %v", err)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Extraer el userId
		userID := claims.UserID
		if userID == "" {
			logger.Log.Warn("[TURN API] Token válido pero sin campo 'sub' (User ID)")
			http.Error(w, "Invalid token payload", http.StatusUnauthorized)
			return
		}

		// Configuración TURN REST API
		turnSecret := cfg.TurnRestApiSecret
		ttl := time.Duration(cfg.TurnCredentialsTTL) * time.Hour
		expireTS := time.Now().Add(ttl).Unix()

		// Generar username: timestamp:userid
		username := fmt.Sprintf("%d:%s", expireTS, userID)

		// Generar password: base64(hmac_sha1(secret, hmac_sha1(username)))
		mac := hmac.New(sha1.New, []byte(turnSecret))
		mac.Write([]byte(username))
		credential := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		// Construir respuesta
		response := ICEServersResponse{
			IceServers: make([]ICECredential, 0),
		}

		// Agregar servidores STUN (no requieren auth)
		if cfg.StunServerUrls != "" {
			stunUrls := strings.Split(cfg.StunServerUrls, ",")
			response.IceServers = append(response.IceServers, ICECredential{
				URLs: stunUrls,
			})
		}

		// Agregar servidores TURN con credenciales generadas
		if cfg.TurnServerUrls != "" {
			turnUrls := strings.Split(cfg.TurnServerUrls, ",")
			response.IceServers = append(response.IceServers, ICECredential{
				URLs:       turnUrls,
				Username:   username,
				Credential: credential,
			})
		}

		logger.Log.Infof("[TURN API] Credenciales generadas para [USER: %s_%s] (expiran en %dh)", claims.Type, claims.UserID, cfg.TurnCredentialsTTL)

		json.NewEncoder(w).Encode(response)
	}
}
