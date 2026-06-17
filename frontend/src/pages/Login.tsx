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
    <div className="flex min-h-screen items-center justify-center bg-stone-100 px-4">
      <div className="w-full max-w-sm rounded-2xl bg-white p-8 shadow-lg">
        <div className="mb-6 text-center">
          <h1 className="text-2xl font-bold text-stone-800">Hugo Blog</h1>
          <p className="mt-1 text-sm text-stone-500">管理界面登录</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="mb-1 block text-sm font-medium text-stone-700">
              用户名
            </label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoFocus
              autoComplete="username"
              className="w-full rounded-lg border border-stone-300 px-3 py-2 text-stone-800 focus:border-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-500"
              placeholder="admin"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-stone-700">
              密码
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              className="w-full rounded-lg border border-stone-300 px-3 py-2 text-stone-800 focus:border-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-500"
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
            className="w-full rounded-lg bg-stone-800 px-4 py-2.5 font-medium text-white transition-colors hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? '登录中…' : '登录'}
          </button>
        </form>
      </div>
    </div>
  );
}
