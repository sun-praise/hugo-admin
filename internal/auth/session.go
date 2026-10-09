// Package auth 提供与 Python 侧兼容的认证能力：
//   - Flask session cookie 的编解码（itsdangerous 签名格式）
//   - data/auth.json 凭据存储（werkzeug 的 scrypt/pbkdf2 哈希格式）
//
// Go 与 Python 服务可互认对方签发的会话与凭据文件，支持灰度并行运行。
package auth

import (
	"bytes"
	"compress/zlib"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	// flaskSalt 是 Flask SecureCookieSessionInterface 固定的盐。
	flaskSalt = "cookie-session"
	// PermanentSessionLifetime 对应 Flask 默认的 31 天。
	PermanentSessionLifetime = 31 * 24 * time.Hour
)

var (
	ErrBadCookie = errors.New("auth: 非法的 session cookie")
	ErrExpired   = errors.New("auth: session cookie 已过期")
)

var b64 = base64.URLEncoding.WithPadding(base64.NoPadding)

type Session map[string]any

// EncodeSession 生成 Flask 兼容的 session cookie 值。
// 注意：map 序列化按键名排序，与 Flask 的插入序可能不同（不影响互读，
// 但签名不同）。需要与 Flask bit 级一致时用 EncodeSessionRaw。
func EncodeSession(secret []byte, s Session, now time.Time) (string, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return EncodeSessionRaw(secret, payload, now)
}

// EncodeSessionRaw 对给定 payload 字节签名，产出 payload_b64.timestamp_b64.sig_b64
// （HMAC-SHA1，itsdangerous 的 key_derivation="hmac" 派生密钥）。
func EncodeSessionRaw(secret, payload []byte, now time.Time) (string, error) {
	payloadB64 := b64.EncodeToString(payload)
	return payloadB64 + "." + timestampPart(secret, payloadB64, now), nil
}

// DecodeSession 校验签名与时效并还原 session 内容。
// maxAge <= 0 表示不做时效检查（仅测试黄金向量时使用）。
func DecodeSession(secret []byte, cookie string, maxAge time.Duration, now time.Time) (Session, error) {
	parts := strings.Split(cookie, ".")
	var signedPayload, rawPayloadB64, tsB64, sigB64 string
	compressed := false
	switch {
	case len(parts) == 3: // 未压缩：payload.ts.sig
		signedPayload, rawPayloadB64, tsB64, sigB64 = parts[0], parts[0], parts[1], parts[2]
	case len(parts) == 4 && parts[0] == "": // zlib 压缩：.payload.ts.sig
		// itsdangerous 的压缩标记 "." 属于被签内容，验签时必须带上
		signedPayload, rawPayloadB64, tsB64, sigB64 = "."+parts[1], parts[1], parts[2], parts[3]
		compressed = true
	default:
		return nil, ErrBadCookie
	}

	if !verifySignature(secret, signedPayload, tsB64, sigB64) {
		return nil, ErrBadCookie
	}

	tsBytes, err := b64.DecodeString(tsB64)
	if err != nil || len(tsBytes) != 4 {
		return nil, ErrBadCookie
	}
	ts := int64(binary.BigEndian.Uint32(tsBytes))
	if maxAge > 0 {
		nowUnix := now.Unix()
		if nowUnix-ts > int64(maxAge/time.Second) || ts > nowUnix+60 {
			return nil, ErrExpired
		}
	}

	payload, err := b64.DecodeString(rawPayloadB64)
	if err != nil {
		return nil, ErrBadCookie
	}
	if compressed {
		zr, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, ErrBadCookie
		}
		defer zr.Close()
		payload, err = io.ReadAll(zr)
		if err != nil {
			return nil, ErrBadCookie
		}
	}

	var s Session
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, ErrBadCookie
	}
	return s, nil
}

// deriveKey 对应 itsdangerous Signer 的 key_derivation="hmac"：
// derived = HMAC-SHA1(secret, salt)。
func deriveKey(secret []byte) []byte {
	mac := hmac.New(sha1.New, secret)
	mac.Write([]byte(flaskSalt))
	return mac.Sum(nil)
}

func timestampPart(secret []byte, payloadB64 string, now time.Time) string {
	ts := make([]byte, 4)
	binary.BigEndian.PutUint32(ts, uint32(now.Unix()))
	tsB64 := b64.EncodeToString(ts)
	mac := hmac.New(sha1.New, deriveKey(secret))
	fmt.Fprintf(mac, "%s.%s", payloadB64, tsB64)
	return tsB64 + "." + b64.EncodeToString(mac.Sum(nil))
}

func verifySignature(secret []byte, payloadB64, tsB64, sigB64 string) bool {
	sig, err := b64.DecodeString(sigB64)
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, deriveKey(secret))
	fmt.Fprintf(mac, "%s.%s", payloadB64, tsB64)
	return hmac.Equal(sig, mac.Sum(nil))
}
