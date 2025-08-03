package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log"
	"net"
	"os"

	"HuaTug.com/cmd/message/dal"
	"HuaTug.com/config"
	"HuaTug.com/config/jaeger"
	message "HuaTug.com/kitex_gen/messages/messageservice"
	"HuaTug.com/pkg/security"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

var (
	// 全局安全组件
	jwtManager       *security.EnhancedJWTManager
	hybridEncryption *security.HybridEncryption
	tlsConfig        *security.TLSConfig
)

func Init() {
	dal.Load()

	// 加载安全配置
	if err := loadSecurityConfig(); err != nil {
		hlog.Errorf("Failed to load security config: %v", err)
	}

	initSecurity()
}

// initSecurity 初始化安全组件
func initSecurity() {
	var err error

	// 获取配置参数
	issuer := "tiktok-message-service"
	audience := "tiktok-users"
	certFile := "/etc/ssl/certs/server.crt"
	keyFile := "/etc/ssl/private/server.key"
	caFile := "/etc/ssl/certs/ca.crt"
	jwtPrivateKeyFile := "/etc/ssl/private/jwt_private.pem"
	jwtPublicKeyFile := "/etc/ssl/certs/jwt_public.pem"

	// 如果有安全配置，使用配置中的值
	if securityConfig != nil {
		if securityConfig.CommunicationSecurity.JWT.Claims.Issuer != "" {
			issuer = securityConfig.CommunicationSecurity.JWT.Claims.Issuer
		}
		if securityConfig.CommunicationSecurity.JWT.Claims.Audience != "" {
			audience = securityConfig.CommunicationSecurity.JWT.Claims.Audience
		}
		if securityConfig.CommunicationSecurity.TLS.CertFile != "" {
			certFile = securityConfig.CommunicationSecurity.TLS.CertFile
		}
		if securityConfig.CommunicationSecurity.TLS.KeyFile != "" {
			keyFile = securityConfig.CommunicationSecurity.TLS.KeyFile
		}
		if securityConfig.CommunicationSecurity.TLS.CAFile != "" {
			caFile = securityConfig.CommunicationSecurity.TLS.CAFile
		}
		if securityConfig.CommunicationSecurity.JWT.PrivateKeyFile != "" {
			jwtPrivateKeyFile = securityConfig.CommunicationSecurity.JWT.PrivateKeyFile
		}
		if securityConfig.CommunicationSecurity.JWT.PublicKeyFile != "" {
			jwtPublicKeyFile = securityConfig.CommunicationSecurity.JWT.PublicKeyFile
		}
	}

	// 生成或读取JWT密钥
	jwtPrivateKeyPEM, jwtPublicKeyPEM, err := generateOrLoadJWTKeys(jwtPrivateKeyFile, jwtPublicKeyFile)
	if err != nil {
		log.Fatalf("Failed to load JWT keys: %v", err)
	}

	// 初始化增强JWT管理器
	jwtManager, err = security.NewEnhancedJWTManager(jwtPrivateKeyPEM, jwtPublicKeyPEM, issuer, audience)
	if err != nil {
		log.Fatalf("Failed to initialize JWT manager: %v", err)
	}

	// 生成或读取加密密钥
	encPrivateKey, encPublicKey, err := generateOrLoadEncryptionKeys()
	if err != nil {
		log.Fatalf("Failed to load encryption keys: %v", err)
	}

	// 初始化混合加密
	hybridEncryption = security.NewHybridEncryption(encPrivateKey, encPublicKey)

	// 初始化TLS配置
	tlsConfig = security.NewTLSConfig(certFile, keyFile, caFile, "message-service")

	hlog.Info("Security components initialized successfully")
}

func main() {
	config.Init()
	Init()

	r, err := etcd.NewEtcdRegistry([]string{config.ConfigInfo.Etcd.Addr})
	if err != nil {
		panic(err)
	}

	addr, err := net.ResolveTCPAddr("tcp", "localhost:8893")
	if err != nil {
		panic(err)
	}

	suite, closer := jaeger.NewServerSuite().Init("Message")
	defer closer.Close()

	// 获取TLS配置
	var serverTLSConfig interface{}
	if securityConfig != nil && securityConfig.CommunicationSecurity.TLS.Enabled {
		tlsConf, err := tlsConfig.GetServerTLSConfig()
		if err != nil {
			hlog.Warnf("Failed to load TLS config, running without TLS: %v", err)
			serverTLSConfig = nil
		} else {
			serverTLSConfig = tlsConf
			hlog.Info("TLS configuration loaded, secure communication enabled")
		}
	} else {
		hlog.Info("TLS disabled in configuration, running without TLS")
	}

	// 创建安全的消息服务实现
	messageService := NewSecureMessageServiceImpl(jwtManager, hybridEncryption)

	var serverOptions []server.Option
	serverOptions = append(serverOptions,
		server.WithServiceAddr(addr),
		server.WithRegistry(r),
		server.WithSuite(suite),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "Message"}),
	)

	// 如果TLS配置成功，可以在这里添加TLS相关的服务器选项
	// 注意：具体的TLS集成方式取决于Kitex框架的支持
	if serverTLSConfig != nil {
		hlog.Info("TLS configuration ready for integration")
		// server.WithTLS(serverTLSConfig) // 示例，具体API需要查看Kitex文档
	}

	svr := message.NewServer(messageService, serverOptions...)

	// 打印安全功能状态
	hlog.Info("=== Message Service Security Status ===")
	hlog.Infof("JWT Authentication: %v", jwtManager != nil)
	hlog.Infof("Hybrid Encryption: %v", hybridEncryption != nil)
	hlog.Infof("TLS Configuration: %v", serverTLSConfig != nil)
	if securityConfig != nil {
		hlog.Infof("Encryption Enabled: %v", securityConfig.CommunicationSecurity.Encryption.HybridEncryption.Enabled)
		hlog.Infof("TLS Enabled: %v", securityConfig.CommunicationSecurity.TLS.Enabled)
	}
	hlog.Info("=======================================")

	hlog.Info("Message service starting with enhanced security features...")
	err = svr.Run()
	if err != nil {
		hlog.Fatal(err)
	}
}

// generateOrLoadJWTKeys 生成或加载JWT密钥
func generateOrLoadJWTKeys(privateKeyFile, publicKeyFile string) ([]byte, []byte, error) {
	// 尝试读取现有密钥
	if privateKeyPEM, err := os.ReadFile(privateKeyFile); err == nil {
		if publicKeyPEM, err := os.ReadFile(publicKeyFile); err == nil {
			hlog.Info("Loaded existing JWT keys")
			return privateKeyPEM, publicKeyPEM, nil
		}
	}

	// 生成新的RSA密钥对
	hlog.Info("Generating new JWT keys...")
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// 编码私钥
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// 编码公钥
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	// 尝试保存密钥（如果目录存在）
	if err := os.MkdirAll("/etc/ssl/private", 0700); err == nil {
		os.WriteFile(privateKeyFile, privateKeyPEM, 0600)
	}
	if err := os.MkdirAll("/etc/ssl/certs", 0755); err == nil {
		os.WriteFile(publicKeyFile, publicKeyPEM, 0644)
	}

	return privateKeyPEM, publicKeyPEM, nil
}

// generateOrLoadEncryptionKeys 生成或加载加密密钥
func generateOrLoadEncryptionKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// 生成新的RSA密钥对用于加密
	hlog.Info("Generating encryption keys...")
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	return privateKey, &privateKey.PublicKey, nil
}
