package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// EnhancedJWTManager 增强的JWT管理器
type EnhancedJWTManager struct {
	privateKey     *rsa.PrivateKey
	publicKey      *rsa.PublicKey
	issuer         string
	audience       string
	keyRotationKey string
	blacklist      map[string]time.Time // 简化实现，生产环境应使用Redis
}

// JWTClaims 增强的JWT声明
type JWTClaims struct {
	UserID      int64    `json:"user_id"`
	Username    string   `json:"username"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	SessionID   string   `json:"session_id"`
	DeviceID    string   `json:"device_id"`
	IPAddress   string   `json:"ip_address"`
	jwt.RegisteredClaims
}

// NewEnhancedJWTManager 创建增强的JWT管理器
func NewEnhancedJWTManager(privateKeyPEM, publicKeyPEM []byte, issuer, audience string) (*EnhancedJWTManager, error) {
	// 解析私钥
	privateKeyBlock, _ := pem.Decode(privateKeyPEM)
	if privateKeyBlock == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBlock.Bytes)
	if err != nil {
		// 尝试PKCS8格式
		parsedKey, err := x509.ParsePKCS8PrivateKey(privateKeyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}
		var ok bool
		privateKey, ok = parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA key")
		}
	}

	// 解析公钥
	publicKeyBlock, _ := pem.Decode(publicKeyPEM)
	if publicKeyBlock == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA key")
	}

	return &EnhancedJWTManager{
		privateKey: privateKey,
		publicKey:  publicKey,
		issuer:     issuer,
		audience:   audience,
		blacklist:  make(map[string]time.Time),
	}, nil
}

// GenerateToken 生成增强的JWT Token
func (jm *EnhancedJWTManager) GenerateToken(claims *JWTClaims, expiration time.Duration) (string, error) {
	now := time.Now()

	// 生成唯一的JTI (JWT ID)
	jti, err := generateSecureRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate JTI: %v", err)
	}

	claims.RegisteredClaims = jwt.RegisteredClaims{
		ID:        jti,
		Issuer:    jm.issuer,
		Audience:  jwt.ClaimStrings{jm.audience},
		Subject:   fmt.Sprintf("%d", claims.UserID),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(expiration)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// 添加额外的头部信息
	token.Header["kid"] = jm.getKeyID() // Key ID for key rotation
	token.Header["alg"] = "RS256"
	token.Header["typ"] = "JWT"

	return token.SignedString(jm.privateKey)
}

// ValidateToken 验证JWT Token
func (jm *EnhancedJWTManager) ValidateToken(tokenString string) (*JWTClaims, error) {
	// 检查黑名单
	if jm.isTokenBlacklisted(tokenString) {
		return nil, fmt.Errorf("token is blacklisted")
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// 验证Key ID（用于密钥轮换）
		if kid, ok := token.Header["kid"].(string); ok {
			if kid != jm.getKeyID() {
				return nil, fmt.Errorf("invalid key ID")
			}
		}

		return jm.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %v", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	// 额外的安全检查
	if err := jm.validateClaims(claims); err != nil {
		return nil, fmt.Errorf("claims validation failed: %v", err)
	}

	return claims, nil
}

// BlacklistToken 将Token加入黑名单
func (jm *EnhancedJWTManager) BlacklistToken(tokenString string) error {
	token, err := jwt.Parse(tokenString, nil)
	if err != nil {
		// 即使解析失败也要加入黑名单
		jm.blacklist[tokenString] = time.Now().Add(24 * time.Hour)
		return nil
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if jti, ok := claims["jti"].(string); ok {
			if exp, ok := claims["exp"].(float64); ok {
				expTime := time.Unix(int64(exp), 0)
				jm.blacklist[jti] = expTime
			}
		}
	}

	return nil
}

// RefreshToken 刷新Token
func (jm *EnhancedJWTManager) RefreshToken(tokenString string) (string, error) {
	claims, err := jm.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// 检查Token是否在刷新窗口内（例如：过期前1小时内）
	if time.Until(claims.ExpiresAt.Time) > time.Hour {
		return "", fmt.Errorf("token is not eligible for refresh")
	}

	// 将旧Token加入黑名单
	jm.BlacklistToken(tokenString)

	// 生成新Token
	newClaims := &JWTClaims{
		UserID:      claims.UserID,
		Username:    claims.Username,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		SessionID:   claims.SessionID,
		DeviceID:    claims.DeviceID,
		IPAddress:   claims.IPAddress,
	}

	return jm.GenerateToken(newClaims, 24*time.Hour)
}

// validateClaims 验证声明
func (jm *EnhancedJWTManager) validateClaims(claims *JWTClaims) error {
	// 验证必要字段
	if claims.UserID <= 0 {
		return fmt.Errorf("invalid user ID")
	}

	if claims.Username == "" {
		return fmt.Errorf("username is required")
	}

	if claims.SessionID == "" {
		return fmt.Errorf("session ID is required")
	}

	// 验证时间声明
	now := time.Now()
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(now) {
		return fmt.Errorf("token is expired")
	}

	if claims.NotBefore != nil && claims.NotBefore.After(now) {
		return fmt.Errorf("token is not valid yet")
	}

	return nil
}

// isTokenBlacklisted 检查Token是否在黑名单中
func (jm *EnhancedJWTManager) isTokenBlacklisted(tokenString string) bool {
	// 简化实现，实际应该使用Redis
	token, err := jwt.Parse(tokenString, nil)
	if err != nil {
		return false
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if jti, ok := claims["jti"].(string); ok {
			if expTime, exists := jm.blacklist[jti]; exists {
				if time.Now().Before(expTime) {
					return true
				}
				// 清理过期的黑名单条目
				delete(jm.blacklist, jti)
			}
		}
	}

	return false
}

// getKeyID 获取密钥ID（用于密钥轮换）
func (jm *EnhancedJWTManager) getKeyID() string {
	// 使用公钥的哈希作为Key ID
	keyBytes, _ := x509.MarshalPKIXPublicKey(jm.publicKey)
	hash := sha256.Sum256(keyBytes)
	return base64.URLEncoding.EncodeToString(hash[:8])
}

// generateSecureRandomString 生成安全的随机字符串
func generateSecureRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// CleanupBlacklist 清理过期的黑名单条目
func (jm *EnhancedJWTManager) CleanupBlacklist() {
	now := time.Now()
	for jti, expTime := range jm.blacklist {
		if now.After(expTime) {
			delete(jm.blacklist, jti)
		}
	}
}
