// Package crypto 提供对 api_key 等敏感字段的本地加密。
//
// 设计：
//   - 主密钥通过"机器指纹"派生（macOS IOPlatformUUID / Linux /etc/machine-id）
//   - 用 scrypt 把指纹扩展成 32 字节 AES-256 key
//   - 加密用 AES-256-GCM（带认证）
//   - 每个 token 独立的随机 salt（16 字节）和 nonce（12 字节）
//   - 输出格式：base64(salt || nonce || ciphertext) 紧凑可放配置文件
//
// 威胁模型：防"误把配置文件 commit 到 git"、"别人拿到你硬盘"等场景。
// 不能防：能跑代码且有 root/sudo 权限的攻击者（他们能直接调 master key 派生函数）。
// 真要更安全，请用 OS Keyring（见 internal/crypto/README.md 的 TODO 讨论）。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize  = 16 // scrypt salt
	nonceSize = 12 // AES-GCM nonce
	keyLen    = 32 // AES-256
	scryptN   = 32768
	scryptR   = 8
	scryptP   = 1
)

// Fingerprint 派生机器指纹（用于主密钥派生）。
//
// macOS: ioreg 拿 IOPlatformUUID
// Linux: /etc/machine-id
// 其他: 返回空字符串 + 错误
func Fingerprint() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err != nil {
			return "", fmt.Errorf("ioreg 失败: %w", err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "IOPlatformUUID") {
				parts := strings.Split(line, `"`)
				if len(parts) >= 4 {
					return strings.TrimSpace(parts[3]), nil
				}
			}
		}
		return "", errors.New("未找到 IOPlatformUUID")
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if data, err := os.ReadFile(p); err == nil {
				return strings.TrimSpace(string(data)), nil
			}
		}
		return "", errors.New("未找到 machine-id")
	default:
		return "", fmt.Errorf("不支持的 OS: %s（暂未实现 fingerprint）", runtime.GOOS)
	}
}

// deriveKey 用 scrypt 从 fingerprint + salt 派生 AES key。
func deriveKey(fingerprint string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(fingerprint), salt, scryptN, scryptR, scryptP, keyLen)
}

// Encrypt 把明文用机器指纹派生的 key 加密，返回 base64 字符串。
//
// 输出格式：base64(salt || nonce || ciphertext)，
// 密文已含 GCM 的 16 字节 tag。
func Encrypt(plaintext string) (string, error) {
	fp, err := Fingerprint()
	if err != nil {
		return "", err
	}
	return encryptWithFingerprint(fp, plaintext)
}

// encryptWithFingerprint 暴露给测试用（可注入稳定指纹）。
func encryptWithFingerprint(fingerprint, plaintext string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key, err := deriveKey(fingerprint, salt)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	// 把密文拼到 salt+nonce 之后
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := make([]byte, 0, len(salt)+len(nonce)+len(ct))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt 是 Encrypt 的逆操作。
func Decrypt(token string) (string, error) {
	fp, err := Fingerprint()
	if err != nil {
		return "", err
	}
	return decryptWithFingerprint(fp, token)
}

func decryptWithFingerprint(fingerprint, token string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}
	if len(data) < saltSize+nonceSize+16 {
		return "", errors.New("密文太短")
	}
	salt := data[:saltSize]
	nonce := data[saltSize : saltSize+nonceSize]
	ct := data[saltSize+nonceSize:]

	key, err := deriveKey(fingerprint, salt)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败（fingerprint 不匹配或密文损坏）: %w", err)
	}
	return string(pt), nil
}

// StableFingerprintHash 返回 fingerprint 的 SHA-256 前 12 字符 hex，
// 用来在 set-key 时简短显示（不暴露完整 UUID）。
// 注意：仅用于人眼辨认，不可作加密材料。
func StableFingerprintHash() string {
	fp, err := Fingerprint()
	if err != nil {
		return "(unknown)"
	}
	sum := sha256.Sum256([]byte(fp))
	return fmt.Sprintf("%x", sum[:6])
}
