package auth

import (
	"strings"
	"testing"
	"time"
)

// 黄金向量由仓库 .venv 的 Flask 3.0.0 实际生成（SECRET_KEY 为
// config.py 开发默认值），保证 Go 编解码与 Python 侧 bit 级一致。
const (
	devSecret     = "dev-secret-key-change-in-production"
	flaskSmall    = "eyJ1c2VybmFtZSI6ImFkbWluIiwiX3Blcm1hbmVudCI6dHJ1ZX0.ashs8A.U2sIwxJfsTYJlup8aN2QEqWMDKQ"
	flaskCompress = ".eJyrViotTi3KS8xNVbJSSkzJzcxT0lFKyslPAnFHAdFAqRYA_XN7XQ.ashteA.BGnSnxAu68tySmFd82rP0XuSLxA"
)

func TestDecodeFlaskCookie(t *testing.T) {
	s, err := DecodeSession([]byte(devSecret), flaskSmall, 0, time.Now())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s["username"] != "admin" || s["_permanent"] != true {
		t.Fatalf("内容不符: %#v", s)
	}
}

func TestDecodeFlaskCompressedCookie(t *testing.T) {
	s, err := DecodeSession([]byte(devSecret), flaskCompress, 0, time.Now())
	if err != nil {
		t.Fatalf("decode compressed: %v", err)
	}
	if s["username"] != "admin" {
		t.Fatalf("内容不符: %#v", s)
	}
	if blob, _ := s["blob"].(string); len(blob) != 300 || strings.Trim(blob, "a") != "" {
		t.Fatalf("blob 内容不符: %d 字节", len(blob))
	}
}

func TestSessionRoundTrip(t *testing.T) {
	now := time.Now()
	value, err := EncodeSession([]byte(devSecret), Session{"username": "go-user", "_permanent": true}, now)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	s, err := DecodeSession([]byte(devSecret), value, PermanentSessionLifetime, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s["username"] != "go-user" {
		t.Fatalf("内容不符: %#v", s)
	}
}

func TestDecodeRejects(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		cookie  string
		secret  string
		maxAge  time.Duration
		at      time.Time
		wantErr error
	}{
		{"篡改签名", strings.Replace(flaskSmall, "U2sI", "X2sI", 1), devSecret, 0, now, ErrBadCookie},
		{"错误密钥", flaskSmall, "another-secret", 0, now, ErrBadCookie},
		{"垃圾值", "not-a-cookie", devSecret, 0, now, ErrBadCookie},
		{"过期", flaskSmall, devSecret, PermanentSessionLifetime, now.Add(40 * 24 * time.Hour), ErrExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := DecodeSession([]byte(c.secret), c.cookie, c.maxAge, c.at); err != c.wantErr {
				t.Fatalf("got %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestEncodeSessionFormat(t *testing.T) {
	// 用 Flask 的原始 payload 字节 + 同一时间戳 ⇒ 签名必须 bit 级一致
	now := time.Unix(1791519984, 0)
	value, err := EncodeSessionRaw([]byte(devSecret),
		[]byte(`{"username":"admin","_permanent":true}`), now)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		t.Fatalf("应为 3 段，得到 %d: %q", len(parts), value)
	}
	if parts[2] != strings.Split(flaskSmall, ".")[2] {
		t.Fatalf("签名与 Flask 不一致: %s vs %s", parts[2], strings.Split(flaskSmall, ".")[2])
	}
}
