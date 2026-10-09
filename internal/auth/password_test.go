package auth

import (
	"strings"
	"testing"
)

// 向量由 .venv 的 werkzeug 3.0.1 generate_password_hash 实际生成。
const (
	scryptVector = "scrypt:32768:8:1$b5iCUCtDnHB6pCsX$00fb23d810ba3053528e5263e3140137a9b3a0014f95973a3c6375774f4a6aa68b3cec6404601c5f3ba1f8322e1c40f66ebd9b239e6f1d62eaaa9bee960a08ee"
	pbkdf2Vector = "pbkdf2:sha256:1000$qEDsdRwm3Cuv5fvq$fdf9bcb981558d5c44fabdf0bc869e06abc76edf979c5497aae993696b6c8745"
	testPassword = "test-password-123"
)

func TestVerifyWerkzeugVectors(t *testing.T) {
	for name, hash := range map[string]string{
		"scrypt": scryptVector,
		"pbkdf2": pbkdf2Vector,
	} {
		if !VerifyPasswordHash(hash, testPassword) {
			t.Fatalf("%s: 正确密码校验失败", name)
		}
		if VerifyPasswordHash(hash, "wrong") {
			t.Fatalf("%s: 错误密码通过校验", name)
		}
	}
}

func TestGeneratePasswordHashFormat(t *testing.T) {
	hash, err := GeneratePasswordHash(testPassword)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "scrypt:32768:8:1") {
		t.Fatalf("格式不符: %q", hash)
	}
	if len(parts[1]) != 16 {
		t.Fatalf("盐长度应为 16: %q", parts[1])
	}
	if !VerifyPasswordHash(hash, testPassword) {
		t.Fatal("自生成哈希无法自校验")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{
		"", "plaintext", "md5$salt$hash", "scrypt$only-two",
		"scrypt:x:8:1$salt$" + "ab",
	} {
		if VerifyPasswordHash(bad, testPassword) {
			t.Fatalf("畸形哈希通过校验: %q", bad)
		}
	}
}
