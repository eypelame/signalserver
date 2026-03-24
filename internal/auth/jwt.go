package auth

import (
	"fmt"
	"time"

	"signalserver/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

// Claims personalizados para el JWT.
type Claims struct {
	UserID   string `json:"sub"`       // Subject, usado como UserID
	UserName string `json:"userName"`  // Nombre del cliente
	Type     string `json:"type"`      // Tipo de usuario: "cliente" o "escucha"
	CallTime int    `json:"call_time"` // Duración máxima de la llamada en minutos (ESTRICTO)
	jwt.RegisteredClaims
}

// JWTValidator define la interfaz para la validación de JWT.
type JWTValidator interface {
	ValidateToken(tokenString string) (*Claims, error)
}

// JWTService implementa JWTValidator.
type JWTService struct {
	secret []byte
	issue  string
}

// NewJWTService crea una nueva instancia de JWTService.
func NewJWTService(cfg *config.AppConfig) *JWTService {
	return &JWTService{
		secret: []byte(cfg.JWTSecret),
		issue:  cfg.JWTIssuer,
	}
}

// ValidateToken valida un token JWT y retorna los claims si es válido.
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verificar el algoritmo de firma
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de firma inesperado: %v", token.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token inválido: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token no válido")
	}

	// Validar Issuer
	if claims.Issuer != s.issue {
		return nil, fmt.Errorf("issuer inválido: esperado %s, obtenido %s", s.issue, claims.Issuer)
	}

	// Validar expiración
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("token expirado")
	}

	// TODO: Añadir validación de Audience (aud) si es necesario

	if err := claims.ValidateBusinessRules(); err != nil {
		return nil, err
	}

	return claims, nil
}

// ValidateBusinessRules aplica reglas de negocio estrictas sobre los claims.
func (c *Claims) ValidateBusinessRules() error {
	if c.Type == "cliente" {
		// Los clientes (callers) DEBEN tener un tiempo de llamada asignado mayor o igual a 0.
		// Si el JSON no traía el campo, sería 0.
		// Sin embargo, la lógica de negocio dice que si es cliente, el token SE GENERÓ para llamar.
		// Si call_time es 0, significa que no tiene saldo. El token es válido criptográficamente,
		// pero funcionalmente no le permitirá hablar.
		// El usuario pide "no opcional".
	} else if c.Type == "escucha" {
		// Los escuchas (listeners) NO DEBEN tener call_time definido (o debe ser 0).
		if c.CallTime != 0 {
			return fmt.Errorf("token de escucha no debe contener call_time (o debe ser 0)")
		}
	}
	return nil
}
