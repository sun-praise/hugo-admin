package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store 是单管理员账户的凭据存储，文件格式与 Python AuthService 一致：
// {"username": "...", "password_hash": "scrypt:...$salt$hex"}。
// 首次启动（文件不存在/为空）引导默认账户；文件损坏则 fail closed。
type Store struct {
	path    string
	mu      sync.Mutex
	account storeAccount
}

type storeAccount struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

var ErrUsernameMismatch = errors.New("用户名不匹配")

// OpenStore 加载或引导凭据文件。
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	acct, exists, err := readAccount(path)
	if err != nil {
		return nil, err
	}
	if exists {
		s.account = acct
		return s, nil
	}

	username := firstNonEmpty(os.Getenv("ADMIN_USERNAME"), "admin")
	password := firstNonEmpty(os.Getenv("ADMIN_PASSWORD"), "admin")
	hash, err := GeneratePasswordHash(password)
	if err != nil {
		return nil, err
	}
	s.account = storeAccount{Username: username, PasswordHash: hash}
	if err := s.write(s.account); err != nil {
		return nil, err
	}
	return s, nil
}

func readAccount(path string) (storeAccount, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return storeAccount{}, false, nil
	}
	if err != nil {
		return storeAccount{}, false, fmt.Errorf("凭据文件不可读 %s: %w", path, err)
	}
	if len(data) == 0 {
		return storeAccount{}, false, nil
	}
	var acct storeAccount
	if err := json.Unmarshal(data, &acct); err != nil {
		return storeAccount{}, false, fmt.Errorf("凭据文件已损坏（JSON 解析失败）%s: %w", path, err)
	}
	if acct.Username == "" || acct.PasswordHash == "" {
		return storeAccount{}, false, fmt.Errorf("凭据文件内容不完整（缺少 username/password_hash）%s", path)
	}
	return acct, true, nil
}

// write 原子写入：临时文件 + rename，避免崩溃损坏凭据文件。
func (s *Store) write(acct storeAccount) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(acct, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".auth_*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *Store) Verify(username, password string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if username == "" || password == "" || username != s.account.Username {
		return false
	}
	return VerifyPasswordHash(s.account.PasswordHash, password)
}

// SetPassword 更新密码并落盘；成功后才更新内存（与 Python 侧语义一致）。
func (s *Store) SetPassword(username, newPassword string) error {
	if newPassword == "" {
		return errors.New("新密码不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if username != s.account.Username {
		return ErrUsernameMismatch
	}
	hash, err := GeneratePasswordHash(newPassword)
	if err != nil {
		return err
	}
	updated := storeAccount{Username: s.account.Username, PasswordHash: hash}
	if err := s.write(updated); err != nil {
		return err
	}
	s.account = updated
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
