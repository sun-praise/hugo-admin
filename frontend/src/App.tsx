import { lazy, Suspense } from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import RequireAuth from './components/RequireAuth';

// Route-level code splitting: each page loads as its own chunk, keeping the
// entry bundle small. Heavy dependencies (highlight.js in Editor's markdown
// renderer, socket.io-client) ship with the page that uses them.
const Login = lazy(() => import('./pages/Login'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const Posts = lazy(() => import('./pages/Posts'));
const Editor = lazy(() => import('./pages/Editor'));
const Server = lazy(() => import('./pages/Server'));
const Git = lazy(() => import('./pages/Git'));
const Plugins = lazy(() => import('./pages/Plugins'));
const Settings = lazy(() => import('./pages/Settings'));

function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* 登录页独立于 Layout，可未登录访问 */}
        <Route path="/login" element={<Suspense fallback={null}><Login /></Suspense>} />
        {/* 其余路由需登录 */}
        <Route element={<RequireAuth />}>
          <Route path="/" element={<Layout />}>
            <Route index element={<Dashboard />} />
            <Route path="posts" element={<Posts />} />
            <Route path="editor" element={<Editor />} />
            <Route path="editor/:filePath/*" element={<Editor />} />
            <Route path="server" element={<Server />} />
            <Route path="git" element={<Git />} />
            <Route path="plugins" element={<Plugins />} />
            <Route path="settings" element={<Settings />} />
          </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
