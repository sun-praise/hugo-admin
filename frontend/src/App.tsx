import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Posts from './pages/Posts';
import Editor from './pages/Editor';
import Server from './pages/Server';
import Settings from './pages/Settings';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="posts" element={<Posts />} />
          <Route path="editor" element={<Editor />} />
          <Route path="editor/:filePath/*" element={<Editor />} />
          <Route path="server" element={<Server />} />
          <Route path="settings" element={<Settings />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
