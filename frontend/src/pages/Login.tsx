import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { login } from '../utils/api';

export default function Login() {
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      const res = await login(username.trim(), password);
      if (res.success) {
        navigate('/', { replace: true });
      } else {
        setError(res.message || '登录失败');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-[#f4f4f5] px-4">
      <div className="w-full max-w-sm animate-rise rounded-[10px] border border-[#e4e4e7] bg-white p-8">
        <div className="mb-6 text-center">
          <h1 className="text-[22px] font-semibold tracking-[-0.02em] text-zinc-900">Hugo Blog</h1>
          <p className="mt-1 text-[13.5px] text-[#52525b]">管理界面登录</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="mb-1 block text-sm font-medium text-[#3f3f46]">
              用户名
            </label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoFocus
              autoComplete="username"
              className="w-full rounded-lg border border-[#e4e4e7] px-3 py-2 text-zinc-900 focus:border-[#d4d4d8] focus:outline-none focus:ring-1 focus:ring-[#d4d4d8]"
              placeholder="admin"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-[#3f3f46]">
              密码
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              className="w-full rounded-lg border border-[#e4e4e7] px-3 py-2 text-zinc-900 focus:border-[#d4d4d8] focus:outline-none focus:ring-1 focus:ring-[#d4d4d8]"
              placeholder="••••••"
            />
          </div>

          {error && (
            <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600">
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={submitting || !username || !password}
            className="btn btn-primary w-full"
          >
            {submitting ? '登录中…' : '登录'}
          </button>
        </form>
      </div>
    </div>
  );
}
