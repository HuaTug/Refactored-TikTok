package main

import (
	"context"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"HuaTug.com/cmd/message/service"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/messages"
	"HuaTug.com/pkg/security"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// SecurityConfig 安全配置结构
type SecurityConfig struct {
	CommunicationSecurity struct {
		TLS struct {
			Enabled    bool   `yaml:"enabled"`
			MinVersion string `yaml:"min_version"`
			MaxVersion string `yaml:"max_version"`
			CertFile   string `yaml:"cert_file"`
			KeyFile    string `yaml:"key_file"`
			CAFile     string `yaml:"ca_file"`
		} `yaml:"tls"`
		JWT struct {
			SigningMethod  string `yaml:"signing_method"`
			PrivateKeyFile string `yaml:"private_key_file"`
			PublicKeyFile  string `yaml:"public_key_file"`
			AccessToken    struct {
				Expiration string `yaml:"expiration"`
			} `yaml:"access_token"`
			Claims struct {
				Issuer   string `yaml:"issuer"`
				Audience string `yaml:"audience"`
			} `yaml:"claims"`
		} `yaml:"jwt"`
		Encryption struct {
			HybridEncryption struct {
				Enabled             bool   `yaml:"enabled"`
				RSAKeySize          int    `yaml:"rsa_key_size"`
				AESKeySize          int    `yaml:"aes_key_size"`
				KeyRotationInterval string `yaml:"key_rotation_interval"`
			} `yaml:"hybrid_encryption"`
		} `yaml:"encryption"`
	} `yaml:"communication_security"`
}

var securityConfig *SecurityConfig

// loadSecurityConfig 加载安全配置
func loadSecurityConfig() error {
	configFile := "config/security.yml"
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		hlog.Warnf("Security config file not found: %s, using defaults", configFile)
		return nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read security config: %v", err)
	}

	securityConfig = &SecurityConfig{}
	if err := yaml.Unmarshal(data, securityConfig); err != nil {
		return fmt.Errorf("failed to parse security config: %v", err)
	}

	hlog.Info("Security configuration loaded successfully")
	return nil
}

type MessageServiceImpl struct{}

// SecureMessageServiceImpl 安全的消息服务实现
type SecureMessageServiceImpl struct {
	securityMiddleware *SecurityMiddleware
}

// NewSecureMessageServiceImpl 创建安全消息服务实例
func NewSecureMessageServiceImpl(jwtManager *security.EnhancedJWTManager, hybridEncryption *security.HybridEncryption) *SecureMessageServiceImpl {
	return &SecureMessageServiceImpl{
		securityMiddleware: NewSecurityMiddleware(jwtManager, hybridEncryption),
	}
}

// InsertMessage implements the SecureMessageServiceImpl interface with security features.
func (s *SecureMessageServiceImpl) InsertMessage(ctx context.Context, request *messages.InsertMessageRequest) (resp *messages.InsertMessageResponse, err error) {
	resp = new(messages.InsertMessageResponse)

	// 1. JWT令牌验证和权限检查
	// 注意：由于thrift定义中没有token字段，这里使用HTTP头或其他方式传递token
	// 在实际应用中，token应该通过HTTP头或RPC元数据传递
	var token string
	// token = getTokenFromContext(ctx) // 从上下文获取token的实现
	var claims *security.JWTClaims
	if token != "" {
		var err error
		claims, err = s.securityMiddleware.AuthenticateRequest(ctx, token)
		if err != nil {
			s.securityMiddleware.LogSecurityEvent("AUTH_FAILED", "unknown", fmt.Sprintf("InsertMessage: %v", err))
			resp.Base = &base.Status{
				Code: consts.StatusUnauthorized,
				Msg:  "Authentication failed",
			}
			return resp, err
		}

		// 权限检查
		if err := s.securityMiddleware.CheckPermission(claims, "message:write"); err != nil {
			s.securityMiddleware.LogSecurityEvent("PERMISSION_DENIED", fmt.Sprintf("%d", claims.UserID),
				fmt.Sprintf("InsertMessage: %v", err))
			resp.Base = &base.Status{
				Code: consts.StatusForbidden,
				Msg:  "Insufficient permissions",
			}
			return resp, err
		}

		// 限流检查
		if err := s.securityMiddleware.CheckRateLimit(fmt.Sprintf("%d", claims.UserID), "insert_message"); err != nil {
			s.securityMiddleware.LogSecurityEvent("RATE_LIMIT_EXCEEDED", fmt.Sprintf("%d", claims.UserID),
				fmt.Sprintf("InsertMessage: %v", err))
			resp.Base = &base.Status{
				Code: consts.StatusTooManyRequests,
				Msg:  "Rate limit exceeded",
			}
			return resp, err
		}
	}

	// 2. 输入验证和消息内容加密
	if request.Message != nil && request.Message.Content != "" {
		// 输入验证
		if err := s.securityMiddleware.ValidateInput(request.Message.Content, 10000); err != nil {
			s.securityMiddleware.LogSecurityEvent("INPUT_VALIDATION_FAILED",
				fmt.Sprintf("%d", getClaimsUserID(claims)), fmt.Sprintf("InsertMessage: %v", err))
			resp.Base = &base.Status{
				Code: consts.StatusBadRequest,
				Msg:  "Invalid input content",
			}
			return resp, err
		}

		// 消息加密（如果启用）
		if securityConfig != nil && securityConfig.CommunicationSecurity.Encryption.HybridEncryption.Enabled {
			encryptedContent, err := s.securityMiddleware.EncryptData([]byte(request.Message.Content))
			if err != nil {
				s.securityMiddleware.LogSecurityEvent("ENCRYPTION_FAILED",
					fmt.Sprintf("%d", getClaimsUserID(claims)), fmt.Sprintf("InsertMessage: %v", err))
				resp.Base = &base.Status{
					Code: consts.StatusInternalServerError,
					Msg:  "Message encryption failed",
				}
				return resp, err
			}

			// 创建加密后的请求
			encryptedRequest := &messages.InsertMessageRequest{
				Message: &messages.MessageInfo{
					FromUid: request.Message.FromUid,
					ToUid:   request.Message.ToUid,
					Content: string(encryptedContent), // 存储加密后的内容
				},
			}

			// 3. 调用原始服务逻辑
			err = service.NewMessageService(ctx).PushMessage(encryptedRequest)
		} else {
			err = service.NewMessageService(ctx).PushMessage(request)
		}
	} else {
		err = service.NewMessageService(ctx).PushMessage(request)
	}

	if err != nil {
		hlog.Errorf("Failed to insert message: %v", err)
		resp.Base = &base.Status{
			Code: consts.StatusBadRequest,
			Msg:  "Fail to insert message",
		}
		return resp, err
	}

	resp.Base = &base.Status{
		Code: consts.StatusOK,
		Msg:  "Success to insert message",
	}

	// 4. 审计日志
	toUid := ""
	if request.Message != nil {
		toUid = request.Message.ToUid
	}
	s.securityMiddleware.LogSecurityEvent("MESSAGE_INSERTED",
		fmt.Sprintf("%d", getClaimsUserID(claims)),
		fmt.Sprintf("to_user=%s, encrypted=%v", toUid,
			securityConfig != nil && securityConfig.CommunicationSecurity.Encryption.HybridEncryption.Enabled))

	return resp, nil
}

// PopMessage implements the SecureMessageServiceImpl interface with security features.
func (s *SecureMessageServiceImpl) PopMessage(ctx context.Context, request *messages.PopMessageRequest) (resp *messages.PopMessageResponse, err error) {
	resp = new(messages.PopMessageResponse)
	resp.Data = &messages.PopMessageResponseData{}

	// 1. JWT令牌验证和权限检查
	// 注意：由于thrift定义中没有token字段，这里使用HTTP头或其他方式传递token
	var token string
	// token = getTokenFromContext(ctx) // 从上下文获取token的实现
	var claims *security.JWTClaims
	if token != "" {
		var err error
		claims, err = s.securityMiddleware.AuthenticateRequest(ctx, token)
		if err != nil {
			s.securityMiddleware.LogSecurityEvent("AUTH_FAILED", "unknown", fmt.Sprintf("PopMessage: %v", err))
			resp.Base = &base.Status{
				Code: consts.StatusUnauthorized,
				Msg:  "Authentication failed",
			}
			return resp, err
		}

		// 权限检查
		if err := s.securityMiddleware.CheckPermission(claims, "message:read"); err != nil {
			s.securityMiddleware.LogSecurityEvent("PERMISSION_DENIED", fmt.Sprintf("%d", claims.UserID),
				fmt.Sprintf("PopMessage: %v", err))
			resp.Base = &base.Status{
				Code: consts.StatusForbidden,
				Msg:  "Insufficient permissions",
			}
			return resp, err
		}

		// 限流检查
		if err := s.securityMiddleware.CheckRateLimit(fmt.Sprintf("%d", claims.UserID), "pop_message"); err != nil {
			s.securityMiddleware.LogSecurityEvent("RATE_LIMIT_EXCEEDED", fmt.Sprintf("%d", claims.UserID),
				fmt.Sprintf("PopMessage: %v", err))
			resp.Base = &base.Status{
				Code: consts.StatusTooManyRequests,
				Msg:  "Rate limit exceeded",
			}
			return resp, err
		}
	}

	// 2. 调用原始服务逻辑获取消息
	data, err := service.NewMessageService(ctx).PopMessage(request)
	if err != nil {
		hlog.Errorf("Failed to pop message: %v", err)
		resp.Base = &base.Status{
			Code: consts.StatusBadRequest,
			Msg:  "Fail to pop message",
		}
		return resp, err
	}

	// 3. 解密消息内容
	if data != nil && len(data.Items) > 0 && securityConfig != nil && securityConfig.CommunicationSecurity.Encryption.HybridEncryption.Enabled {
		for i, msg := range data.Items {
			if msg.Content != "" {
				// 尝试解密消息内容
				decryptedContent, err := s.securityMiddleware.DecryptData([]byte(msg.Content))
				if err != nil {
					// 如果解密失败，可能是未加密的消息，保持原样
					hlog.Warnf("Failed to decrypt message content, keeping original: %v", err)
					continue
				}
				// 更新解密后的内容
				data.Items[i].Content = string(decryptedContent)
			}
		}
	}

	resp.Base = &base.Status{
		Code: consts.StatusOK,
		Msg:  "Success to pop message",
	}
	resp.Data = data

	// 4. 审计日志
	messageCount := 0
	if data != nil && data.Items != nil {
		messageCount = len(data.Items)
	}
	s.securityMiddleware.LogSecurityEvent("MESSAGES_RETRIEVED",
		fmt.Sprintf("%d", getClaimsUserID(claims)),
		fmt.Sprintf("user_id=%s, count=%d", request.Uid, messageCount))

	return resp, nil
}

// getClaimsUserID 获取用户ID的辅助函数
func getClaimsUserID(claims *security.JWTClaims) int64 {
	if claims == nil {
		return 0
	}
	return claims.UserID
}

// getTokenFromContext 从上下文获取JWT令牌的辅助函数
// 在实际应用中，token应该通过RPC元数据或HTTP头传递
func getTokenFromContext(ctx context.Context) string {
	// 这里是示例实现，实际应该从Kitex的元数据中获取
	// 例如：
	// if md, ok := metainfo.FromIncomingContext(ctx); ok {
	//     if token, exists := md["authorization"]; exists {
	//         return strings.TrimPrefix(token, "Bearer ")
	//     }
	// }
	return "" // 暂时返回空字符串，实际使用时需要实现
}

// 保持原有的简单实现作为备用
// InsertMessage implements the MessageServiceImpl interface.
func (s *MessageServiceImpl) InsertMessage(ctx context.Context, request *messages.InsertMessageRequest) (resp *messages.InsertMessageResponse, err error) {
	resp = new(messages.InsertMessageResponse)

	err = service.NewMessageService(ctx).PushMessage(request)
	if err != nil {
		resp.Base = &base.Status{
			Code: consts.StatusBadRequest,
			Msg:  "Fail to insert message",
		}
		return resp, err
	}

	resp.Base = &base.Status{
		Code: consts.StatusOK,
		Msg:  "Success to insert message",
	}
	return resp, nil
}

// PopMessage implements the MessageServiceImpl interface.
func (s *MessageServiceImpl) PopMessage(ctx context.Context, request *messages.PopMessageRequest) (resp *messages.PopMessageResponse, err error) {
	resp = new(messages.PopMessageResponse)
	resp.Data = &messages.PopMessageResponseData{}

	data, err := service.NewMessageService(ctx).PopMessage(request)
	if err != nil {
		resp.Base = &base.Status{
			Code: consts.StatusBadRequest,
			Msg:  "Fail to pop message",
		}
		return resp, err
	}

	resp.Base = &base.Status{
		Code: consts.StatusOK,
		Msg:  "Success to pop message",
	}
	resp.Data = data
	return resp, nil
}
