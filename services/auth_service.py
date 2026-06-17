# coding: utf-8
"""
基于密码验证的用户认证服务。

凭据存储在仓库安装目录下的 ``data/auth.json``（与 HUGO_ROOT 无关，
切换博客仓库不会丢失登录账户）。密码仅以 werkzeug 自带的自描述盐值
哈希保存，绝不存明文。
"""

import json
import os
import tempfile
from pathlib import Path
from typing import Optional

from werkzeug.security import check_password_hash, generate_password_hash


class AuthStoreError(RuntimeError):
    """凭据存储损坏/不可读/不完整时抛出。

    采用 fail closed：绝不静默回退到默认管理员（那会把一次磁盘/权限/
    手工编辑错误变成认证弱化）。该异常会在应用启动时上抛，阻止以不安全
    状态启动。
    """


class AuthService:
    """单管理员账户的密码认证服务。"""

    def __init__(
        self,
        store_path,
        default_username: Optional[str] = None,
        default_password: Optional[str] = None,
    ):
        """
        :param store_path: 凭据 JSON 文件路径。
        :param default_username: 首次启动时使用的默认用户名（None 则读环境变量/回退 admin）。
        :param default_password: 首次启动时使用的默认密码（None 则读环境变量/回退 admin）。
        """
        self.store_path = Path(store_path)
        self.store_path.parent.mkdir(parents=True, exist_ok=True)
        self._default_username = default_username
        self._default_password = default_password
        self._account = self._bootstrap_or_load()

    # ============ 加载 / 首次启动引导 ============

    def _bootstrap_or_load(self) -> dict:
        """加载已存在账户；仅在文件不存在/为空（首次启动）时引导默认账户。

        - 文件不存在或为空 → 创建默认账户（首次启动）。
        - 文件存在且合法 → 原样加载，绝不覆盖。
        - 文件存在但损坏/不可读/字段缺失 → 抛 ``AuthStoreError``（fail closed）。
        """
        account = self._read_account()  # None | dict | raises
        if account is not None:
            return account

        username = self._default_username or os.environ.get("ADMIN_USERNAME") or "admin"
        password = self._default_password or os.environ.get("ADMIN_PASSWORD") or "admin"
        password_hash = generate_password_hash(password)
        self._write({"username": username, "password_hash": password_hash})

        from_env = bool(self._default_password or os.environ.get("ADMIN_PASSWORD"))
        if from_env:
            print(
                f"⚠ 已创建默认管理员账户 用户名={username}"
                "（密码来自 ADMIN_PASSWORD）。建议登录后尽快修改密码。"
            )
        else:
            print(
                f"⚠ 已创建默认管理员账户 用户名={username} 密码={password}"
                "（默认弱密码，不安全！）。请设置环境变量 ADMIN_PASSWORD，"
                "或登录后在应用内修改密码。"
            )
        return {"username": username, "password_hash": password_hash}

    def _read_account(self) -> Optional[dict]:
        """读取并校验账户文件。

        - 文件不存在或为空 → 返回 ``None``（视为首次启动）。
        - 文件存在但不可读 / JSON 非法 / 缺少必要字段 → 抛 ``AuthStoreError``。
        """
        if not self.store_path.exists():
            return None
        try:
            raw = self.store_path.read_text(encoding="utf-8")
        except OSError as e:
            raise AuthStoreError(f"凭据文件不可读 {self.store_path}: {e}") from e
        if not raw.strip():
            return None  # 空文件 == 首次启动
        try:
            data = json.loads(raw)
        except json.JSONDecodeError as e:
            raise AuthStoreError(
                f"凭据文件已损坏（JSON 解析失败）{self.store_path}: {e}"
            ) from e
        if (
            not isinstance(data, dict)
            or not data.get("username")
            or not data.get("password_hash")
        ):
            raise AuthStoreError(
                f"凭据文件内容不完整（缺少 username/password_hash）{self.store_path}"
            )
        return data

    # ============ 写入（原子） ============

    def _write(self, account: dict):
        """原子写入：先写临时文件再 os.replace，避免崩溃损坏凭据文件。"""
        self.store_path.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp_path = tempfile.mkstemp(
            prefix=".auth_", suffix=".tmp", dir=str(self.store_path.parent)
        )
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                json.dump(account, f, ensure_ascii=False, indent=2)
            os.replace(tmp_path, self.store_path)
        except Exception:
            try:
                os.unlink(tmp_path)
            except OSError:
                pass
            raise

    # ============ 对外 API ============

    def get_user(self) -> Optional[dict]:
        """返回当前账户的公开信息（不含密码哈希）。"""
        if not self._account:
            return None
        return {"username": self._account.get("username")}

    def verify(self, username: str, password: str) -> bool:
        """校验用户名 + 密码。"""
        if not username or not password or not self._account:
            return False
        if username != self._account.get("username"):
            return False
        pwhash = self._account.get("password_hash", "")
        if not pwhash:
            return False
        return check_password_hash(pwhash, password)

    def set_password(self, username: str, new_password: str):
        """更新指定账户的密码（重新哈希）。需严格匹配用户名。"""
        if not new_password:
            raise ValueError("新密码不能为空")
        if not self._account:
            raise ValueError("账户不存在")
        if username != self._account.get("username"):
            raise ValueError("用户名不匹配")
        self._account["password_hash"] = generate_password_hash(new_password)
        self._write(self._account)
