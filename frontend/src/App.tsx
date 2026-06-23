import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import RequireAuth from './components/RequireAuth';
import Dashboard from './pages/Dashboard';
import Posts from './pages/Posts';
import Editor from './pages/Editor';
import Server from './pages/Server';
import Settings from './pages/Settings';
import ProjectInit from './pages/ProjectInit';
import Plugins from './pages/Plugins';
import Git from './pages/Git';
import Login from './pages/Login';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* 登录页独立于 Layout，可未登录访问 */}
        <Route path="/login" element={<Login />} />
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
            <Route path="project-init" element={<ProjectInit />} />
          </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
