package sync_test

import (
	"bytes"
	"testing"

	"tally/internal/sync"
)

func TestCryptoRoundTrip(t *testing.T) {
	password := "SecretPass123!"
	plaintext := []byte("Tally SQLite database snapshot binary test content 1234567890")

	salt, err := sync.GenerateRandomBytes(sync.SaltLen)
	if err != nil {
		t.Fatalf("GenerateRandomBytes(salt) failed: %v", err)
	}
	nonce, err := sync.GenerateRandomBytes(sync.NonceLen)
	if err != nil {
		t.Fatalf("GenerateRandomBytes(nonce) failed: %v", err)
	}

	key, err := sync.DeriveKey(password, salt)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	container, err := sync.EncryptBlock(plaintext, key, nonce)
	if err != nil {
		t.Fatalf("EncryptBlock failed: %v", err)
	}

	// 验证文件头
	if !bytes.HasPrefix(container, []byte("TALLYENC1\x01\x01")) {
		t.Fatalf("unexpected container prefix: %x", container[:11])
	}

	expectedHash := sync.SHA256String(plaintext)

	// 正确解密
	decrypted, err := sync.DecryptBlock(container, key, nonce, expectedHash)
	if err != nil {
		t.Fatalf("DecryptBlock failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted text does not match plaintext")
	}

	// 错误密码解密
	wrongKey, _ := sync.DeriveKey("WrongPassword", salt)
	_, err = sync.DecryptBlock(container, wrongKey, nonce, expectedHash)
	if err == nil {
		t.Fatalf("expected decryption failure with wrong key, got nil")
	}

	// 篡改密文
	corrupted := append([]byte(nil), container...)
	corrupted[len(corrupted)-1] ^= 0xFF
	_, err = sync.DecryptBlock(corrupted, key, nonce, expectedHash)
	if err == nil {
		t.Fatalf("expected decryption failure with corrupted ciphertext, got nil")
	}

	// 校验哈希不匹配
	_, err = sync.DecryptBlock(container, key, nonce, "0000000000000000000000000000000000000000000000000000000000000000")
	if err != sync.ErrHashMismatch {
		t.Fatalf("expected ErrHashMismatch, got %v", err)
	}
}
