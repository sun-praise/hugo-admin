package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

// werkzeug 的 gen_salt 字符集：string.ascii_letters + string.digits。
const saltCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

const (
	defaultScryptN = 1 << 15 // 32768，werkzeug 默认
	defaultScryptR = 8
	defaultScryptP = 1
	scryptKeyLen   = 64 // hashlib.scrypt 默认 dklen
	saltLength     = 16
)

// GeneratePasswordHash 生成 werkzeug 兼容的 scrypt 哈希：
// scrypt:N:R:P$salt$hex，Python 侧 check_password_hash 可直接校验。
func GeneratePasswordHash(password string) (string, error) {
	salt, err := genSalt(saltLength)
	if err != nil {
		return "", err
	}
	dk, err := scrypt.Key([]byte(password), []byte(salt), defaultScryptN, defaultScryptR, defaultScryptP, scryptKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("scrypt:%d:%d:%d$%s$%s",
		defaultScryptN, defaultScryptR, defaultScryptP, salt, hex.EncodeToString(dk)), nil
}

// VerifyPasswordHash 校验 werkzeug generate_password_hash 生成的哈希，
// 支持 scrypt 与 pbkdf2 两种格式。
func VerifyPasswordHash(pwhash, password string) bool {
	parts := strings.Split(pwhash, "$")
	if len(parts) != 3 {
		return false
	}
	method, salt, want := parts[0], parts[1], parts[2]

	name, args, _ := strings.Cut(method, ":")
	switch name {
	case "scrypt":
		n, r, p := defaultScryptN, defaultScryptR, defaultScryptP
		if len(args) > 0 {
			fields := strings.Split(args, ":")
			if len(fields) != 3 {
				return false
			}
			var err error
			if n, err = strconv.Atoi(fields[0]); err != nil {
				return false
			}
			if r, err = strconv.Atoi(fields[1]); err != nil {
				return false
			}
			if p, err = strconv.Atoi(fields[2]); err != nil {
				return false
			}
		}
		dk, err := scrypt.Key([]byte(password), []byte(salt), n, r, p, scryptKeyLen)
		return err == nil && hex.EncodeToString(dk) == want
	case "pbkdf2":
		hashName, iterations := "sha256", 600000
		if len(args) > 0 {
			fields := strings.Split(args, ":")
			if len(fields) > 2 {
				return false
			}
			hashName = fields[0]
			if len(fields) == 2 {
				var err error
				if iterations, err = strconv.Atoi(fields[1]); err != nil {
					return false
				}
			}
		}
		digest, ok := digestByName(hashName)
		if !ok {
			return false
		}
		dk := pbkdf2.Key([]byte(password), []byte(salt), iterations, digest().Size(), digest)
		return hex.EncodeToString(dk) == want
	default:
		return false
	}
}

// digestByName 返回哈希构造函数（pbkdf2 要求每次调用产生新实例）。
func digestByName(name string) (func() hash.Hash, bool) {
	switch strings.ToLower(name) {
	case "sha256":
		return sha256.New, true
	default: // 现有凭据只可能是 sha256；其余哈希按需再补
		return nil, false
	}
}

func genSalt(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = saltCharset[int(b)%len(saltCharset)]
	}
	return string(out), nil
}
