package crypto

import (
	"strings"
	"testing"
)

// TestRoundTrip 验证同一指纹下 Encrypt/Decrypt 互逆。
func TestRoundTrip(t *testing.T) {
	const fp = "test-fingerprint-12345"
	plain := "sk-ant-api03-very-secret-token-abcdef"
	ct, err := encryptWithFingerprint(fp, plain)
	if err != nil {
		t.Fatal(err)
	}
	if ct == plain {
		t.Error("密文应与明文不同")
	}
	got, err := decryptWithFingerprint(fp, ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("解密后不相等: got %q, want %q", got, plain)
	}
}

// TestDifferentFingerprintFails 验证不同指纹无法解密。
func TestDifferentFingerprintFails(t *testing.T) {
	ct, err := encryptWithFingerprint("machine-A", "secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = decryptWithFingerprint("machine-B", ct)
	if err == nil {
		t.Fatal("不同指纹不应能解密")
	}
	if !strings.Contains(err.Error(), "fingerprint 不匹配") {
		t.Errorf("错误信息应指明 fingerprint 错配: %v", err)
	}
}

// TestCorruptedCiphertextFails 验证密文被破坏后无法解密。
func TestCorruptedCiphertextFails(t *testing.T) {
	ct, _ := encryptWithFingerprint("fp", "secret")
	// 篡改最后几个字节（tag 验证会失败）
	corrupted := ct[:len(ct)-4] + "AAAA"
	_, err := decryptWithFingerprint("fp", corrupted)
	if err == nil {
		t.Error("篡改后的密文不应能解密")
	}
}

// TestInvalidBase64Fails 验证非法 base64 报错。
func TestInvalidBase64Fails(t *testing.T) {
	_, err := decryptWithFingerprint("fp", "!!! not base64 !!!")
	if err == nil {
		t.Error("非法 base64 应报错")
	}
}

// TestTooShortFails 验证密文过短报错。
func TestTooShortFails(t *testing.T) {
	// 远低于 saltSize+nonceSize+16 的 base64
	short := "AAAA"
	_, err := decryptWithFingerprint("fp", short)
	if err == nil {
		t.Error("过短密文应报错")
	}
}

// TestStableFingerprintHash 验证 fingerprint hash 函数不 panic。
func TestStableFingerprintHash(t *testing.T) {
	h := StableFingerprintHash()
	if h == "" {
		t.Error("应返回非空 hash")
	}
}

// TestEncryptProducesDifferentOutputs 验证相同明文 + 相同指纹，输出不同（因为 salt/nonce 随机）。
func TestEncryptProducesDifferentOutputs(t *testing.T) {
	a, _ := encryptWithFingerprint("fp", "same plaintext")
	b, _ := encryptWithFingerprint("fp", "same plaintext")
	if a == b {
		t.Error("每次加密输出应不同（salt/nonce 随机）")
	}
}
