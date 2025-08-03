package security

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// HybridEncryption 混合加密服务
type HybridEncryption struct {
	rsaPrivateKey *rsa.PrivateKey
	rsaPublicKey  *rsa.PublicKey
	keyVersion    int
	rotationTime  time.Time
}

// EncryptedMessage 加密消息结构
type EncryptedMessage struct {
	EncryptedAESKey []byte `json:"encrypted_aes_key"`
	EncryptedData   []byte `json:"encrypted_data"`
	Nonce           []byte `json:"nonce"`
	KeyVersion      int    `json:"key_version"`
	Timestamp       int64  `json:"timestamp"`
	Signature       []byte `json:"signature"`
}

// NewHybridEncryption 创建混合加密服务
func NewHybridEncryption(privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey) *HybridEncryption {
	return &HybridEncryption{
		rsaPrivateKey: privateKey,
		rsaPublicKey:  publicKey,
		keyVersion:    1,
		rotationTime:  time.Now(),
	}
}

// GetPublicKey 获取公钥
func (he *HybridEncryption) GetPublicKey() *rsa.PublicKey {
	return he.rsaPublicKey
}

// GetPrivateKey 获取私钥
func (he *HybridEncryption) GetPrivateKey() *rsa.PrivateKey {
	return he.rsaPrivateKey
}

// Encrypt 混合加密（RSA + AES-GCM）
func (he *HybridEncryption) Encrypt(plaintext []byte, recipientPublicKey *rsa.PublicKey) (*EncryptedMessage, error) {
	// 1. 生成随机AES密钥
	aesKey := make([]byte, 32) // AES-256
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %v", err)
	}

	// 2. 使用RSA加密AES密钥
	encryptedAESKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, recipientPublicKey, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt AES key: %v", err)
	}

	// 3. 使用AES-GCM加密数据
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %v", err)
	}

	// 生成随机nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %v", err)
	}

	// 加密数据
	encryptedData := gcm.Seal(nil, nonce, plaintext, nil)

	// 4. 创建消息结构
	message := &EncryptedMessage{
		EncryptedAESKey: encryptedAESKey,
		EncryptedData:   encryptedData,
		Nonce:           nonce,
		KeyVersion:      he.keyVersion,
		Timestamp:       time.Now().Unix(),
	}

	// 5. 生成数字签名
	signature, err := he.signMessage(message)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %v", err)
	}
	message.Signature = signature

	return message, nil
}

// Decrypt 混合解密
func (he *HybridEncryption) Decrypt(message *EncryptedMessage, senderPublicKey *rsa.PublicKey) ([]byte, error) {
	// 1. 验证数字签名
	if err := he.verifySignature(message, senderPublicKey); err != nil {
		return nil, fmt.Errorf("signature verification failed: %v", err)
	}

	// 2. 检查时间戳（防重放攻击）
	if time.Now().Unix()-message.Timestamp > 300 { // 5分钟窗口
		return nil, fmt.Errorf("message timestamp is too old")
	}

	// 3. 使用RSA解密AES密钥
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, he.rsaPrivateKey, message.EncryptedAESKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %v", err)
	}

	// 4. 使用AES-GCM解密数据
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %v", err)
	}

	// 解密数据
	plaintext, err := gcm.Open(nil, message.Nonce, message.EncryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %v", err)
	}

	return plaintext, nil
}

// signMessage 对消息进行数字签名
func (he *HybridEncryption) signMessage(message *EncryptedMessage) ([]byte, error) {
	// 创建消息摘要
	hash := sha256.New()
	hash.Write(message.EncryptedAESKey)
	hash.Write(message.EncryptedData)
	hash.Write(message.Nonce)
	hash.Write([]byte(fmt.Sprintf("%d%d", message.KeyVersion, message.Timestamp)))

	hashed := hash.Sum(nil)

	// 使用RSA私钥签名
	signature, err := rsa.SignPKCS1v15(rand.Reader, he.rsaPrivateKey, crypto.SHA256, hashed)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// verifySignature 验证数字签名
func (he *HybridEncryption) verifySignature(message *EncryptedMessage, publicKey *rsa.PublicKey) error {
	// 重新计算消息摘要
	hash := sha256.New()
	hash.Write(message.EncryptedAESKey)
	hash.Write(message.EncryptedData)
	hash.Write(message.Nonce)
	hash.Write([]byte(fmt.Sprintf("%d%d", message.KeyVersion, message.Timestamp)))

	hashed := hash.Sum(nil)

	// 验证签名
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed, message.Signature)
}

// SecureKeyExchange 安全密钥交换
type SecureKeyExchange struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

// NewSecureKeyExchange 创建安全密钥交换
func NewSecureKeyExchange(privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey) *SecureKeyExchange {
	return &SecureKeyExchange{
		privateKey: privateKey,
		publicKey:  publicKey,
	}
}

// GenerateSharedSecret 生成共享密钥
func (ske *SecureKeyExchange) GenerateSharedSecret(peerPublicKey *rsa.PublicKey) ([]byte, error) {
	// 生成随机种子
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}

	// 使用对方公钥加密种子
	encryptedSeed, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, peerPublicKey, seed, nil)
	if err != nil {
		return nil, err
	}

	// 使用PBKDF2派生共享密钥
	salt := make([]byte, 16)
	rand.Read(salt)

	sharedSecret := pbkdf2.Key(encryptedSeed, salt, 100000, 32, sha256.New)

	return sharedSecret, nil
}

// SecureCompare 安全比较（防时序攻击）
func SecureCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// SecureRandom 生成安全随机数
func SecureRandom(length int) ([]byte, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

// Base64Encode 安全的Base64编码
func Base64Encode(data []byte) string {
	return base64.URLEncoding.EncodeToString(data)
}

// Base64Decode 安全的Base64解码
func Base64Decode(encoded string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(encoded)
}
