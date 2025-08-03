package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"time"
)

// TLSConfig TLS配置管理器
type TLSConfig struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	ServerName string
}

// NewTLSConfig 创建TLS配置
func NewTLSConfig(certFile, keyFile, caFile, serverName string) *TLSConfig {
	return &TLSConfig{
		CertFile:   certFile,
		KeyFile:    keyFile,
		CAFile:     caFile,
		ServerName: serverName,
	}
}

// GetServerTLSConfig 获取服务端TLS配置
func (tc *TLSConfig) GetServerTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(tc.CertFile, tc.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificates: %v", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12, // 强制使用TLS 1.2+
		MaxVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			// TLS 1.3 cipher suites (automatically selected)
			// TLS 1.2 cipher suites
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
		PreferServerCipherSuites: true,
		SessionTicketsDisabled:   false,
		SessionTicketKey:         [32]byte{}, // 定期轮换
	}

	// 如果提供了CA文件，启用客户端证书验证
	if tc.CAFile != "" {
		caCert, err := ioutil.ReadFile(tc.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		config.ClientCAs = caCertPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return config, nil
}

// GetClientTLSConfig 获取客户端TLS配置
func (tc *TLSConfig) GetClientTLSConfig() (*tls.Config, error) {
	config := &tls.Config{
		ServerName: tc.ServerName,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
		InsecureSkipVerify: false, // 生产环境必须验证证书
	}

	// 如果提供了客户端证书
	if tc.CertFile != "" && tc.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tc.CertFile, tc.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificates: %v", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	// 如果提供了CA文件
	if tc.CAFile != "" {
		caCert, err := ioutil.ReadFile(tc.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		config.RootCAs = caCertPool
	}

	return config, nil
}

// CertificateRotator 证书轮换器
type CertificateRotator struct {
	tlsConfig     *TLSConfig
	rotateTimer   *time.Timer
	rotateFunc    func() error
	checkInterval time.Duration
}

// NewCertificateRotator 创建证书轮换器
func NewCertificateRotator(tlsConfig *TLSConfig, checkInterval time.Duration) *CertificateRotator {
	return &CertificateRotator{
		tlsConfig:     tlsConfig,
		checkInterval: checkInterval,
	}
}

// StartRotation 启动证书轮换
func (cr *CertificateRotator) StartRotation() {
	cr.rotateTimer = time.NewTimer(cr.checkInterval)
	go func() {
		for {
			select {
			case <-cr.rotateTimer.C:
				if err := cr.checkAndRotate(); err != nil {
					// Log error but continue
					fmt.Printf("Certificate rotation check failed: %v\n", err)
				}
				cr.rotateTimer.Reset(cr.checkInterval)
			}
		}
	}()
}

// checkAndRotate 检查并轮换证书
func (cr *CertificateRotator) checkAndRotate() error {
	// 检查证书是否即将过期（30天内）
	cert, err := tls.LoadX509KeyPair(cr.tlsConfig.CertFile, cr.tlsConfig.KeyFile)
	if err != nil {
		return err
	}

	if len(cert.Certificate) == 0 {
		return fmt.Errorf("no certificates found")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return err
	}

	// 如果证书在30天内过期，触发轮换
	if time.Until(x509Cert.NotAfter) < 30*24*time.Hour {
		if cr.rotateFunc != nil {
			return cr.rotateFunc()
		}
	}

	return nil
}

// StopRotation 停止证书轮换
func (cr *CertificateRotator) StopRotation() {
	if cr.rotateTimer != nil {
		cr.rotateTimer.Stop()
	}
}
