// Package sync 实现基于 WebDAV 的加密数据同步与跨设备协作。
package sync

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	MagicHeader   = "TALLYENC1" // 9 bytes
	FormatVersion = 0x01
	AlgoAESGCM    = 0x01

	ScryptN  = 32768
	ScryptR  = 8
	ScryptP  = 1
	KeyLen   = 32
	SaltLen  = 16
	NonceLen = 12
)

var (
	ErrInvalidHeader     = errors.New("无效的加密容器文件头")
	ErrUnsupportedFormat = errors.New("不支持的加密版本或算法")
	ErrDecryptionFailed  = errors.New("数据包解密失败，请检查同步密码是否正确")
	ErrHashMismatch      = errors.New("明文哈希校验不匹配")
)

// DeriveKey 使用 scrypt 从用户密码与 salt 派生 32 字节密钥。
func DeriveKey(password string, salt []byte) ([]byte, error) {
	if len(password) == 0 {
		return nil, errors.New("加密密码不能为空")
	}
	if len(salt) == 0 {
		return nil, errors.New("salt 不能为空")
	}
	return scrypt.Key([]byte(password), salt, ScryptN, ScryptR, ScryptP, KeyLen)
}

// GenerateRandomBytes 生成指定长度的强随机字节。
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, fmt.Errorf("生成随机数失败: %w", err)
	}
	return b, nil
}

// EncryptBlock 使用 AES-256-GCM 加密明文并打包为 TALLYENC1 容器格式。
func EncryptBlock(plaintext []byte, key, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce 长度必须为 %d 字节", gcm.NonceSize())
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	var buf bytes.Buffer
	buf.WriteString(MagicHeader)
	buf.WriteByte(FormatVersion)
	buf.WriteByte(AlgoAESGCM)
	buf.Write(ciphertext)
	return buf.Bytes(), nil
}

// DecryptBlock 解密 TALLYENC1 格式容器数据，并校验可选的预期 SHA-256 哈希。
func DecryptBlock(container []byte, key, nonce []byte, expectedHash string) ([]byte, error) {
	headerLen := len(MagicHeader) + 2
	if len(container) < headerLen+16 {
		return nil, ErrInvalidHeader
	}
	if !bytes.Equal(container[:len(MagicHeader)], []byte(MagicHeader)) {
		return nil, ErrInvalidHeader
	}
	if container[len(MagicHeader)] != FormatVersion || container[len(MagicHeader)+1] != AlgoAESGCM {
		return nil, ErrUnsupportedFormat
	}
	ciphertext := container[headerLen:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce 长度必须为 %d 字节", gcm.NonceSize())
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}
	if expectedHash != "" {
		hash := SHA256String(plaintext)
		if hash != expectedHash {
			return nil, ErrHashMismatch
		}
	}
	return plaintext, nil
}

// SHA256String 计算数据的 SHA-256 16 进制小写字符串。
func SHA256String(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
