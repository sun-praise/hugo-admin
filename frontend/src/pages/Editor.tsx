import { useState, useEffect, useRef } from 'react';
import { useParams, Link } from 'react-router-dom';
import {
  Save,
  Upload,
  FileCode,
  Link2,
  Image,
  Bold,
  Italic,
  Heading,
  List,
  ListOrdered,
  Quote,
  Code,
  Table,
  X,
  ArrowLeftRight,
} from 'lucide-react';
import { get, post } from '../utils/api';
import { renderMarkdown } from '../utils/markdown';
import type { FileData, ImageItem, Backlink, Frontmatter } from '../types';

export default function Editor() {
  const { filePath, '*': restPath } = useParams();
  // navigate not used in this page
  const fullPath = restPath ? `${filePath}/${restPath}` : filePath || '';

  const [content, setContent] = useState('');
  const [originalContent, setOriginalContent] = useState('');
  const [preview, setPreview] = useState('');
  const [hasChanges, setHasChanges] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [isPublished, setIsPublished] = useState(false);
  const [showImageManager, setShowImageManager] = useState(false);
  const [showFrontmatterDrawer, setShowFrontmatterDrawer] = useState(false);
  const [showBacklinks, setShowBacklinks] = useState(false);
  const [backlinks, setBacklinks] = useState<Backlink[]>([]);
  const [frontmatter, setFrontmatter] = useState<Frontmatter>({});
  const [originalFrontmatter, setOriginalFrontmatter] = useState<Frontmatter>({});
  const [fmEdit, setFmEdit] = useState<Record<string, string>>({});
  const [fmTagsStr, setFmTagsStr] = useState('');
  const [fmCategoriesStr, setFmCategoriesStr] = useState('');
  const [fmDateLocal, setFmDateLocal] = useState('');
  const [fmExtraFields, setFmExtraFields] = useState<Array<{ key: string; value: string }>>([]);
  const [images, setImages] = useState<ImageItem[]>([]);
  const [showRefModal, setShowRefModal] = useState(false);
  const [refSearchQuery, setRefSearchQuery] = useState('');
  const [refSearchResults, setRefSearchResults] = useState<Array<{ path: string; title: string }>>([]);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const currentFile = fullPath;

  useEffect(() => {
    if (currentFile) {
      loadFile();
      loadImages();
      loadBacklinks();
    }
  }, [currentFile]);

  useEffect(() => {
    updatePreview();
  }, [content, currentFile]);

  useEffect(() => {
    setHasChanges(
      content !== originalContent || JSON.stringify(frontmatter) !== JSON.stringify(originalFrontmatter),
    );
  }, [content, originalContent, frontmatter, originalFrontmatter]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && showRefModal) {
        setShowRefModal(false);
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasChanges && !saving) saveFile();
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'b') {
        e.preventDefault();
        insertMarkdown('bold');
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'i') {
        e.preventDefault();
        insertMarkdown('italic');
      }
    }
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [hasChanges, saving, showRefModal]);

  function syncFmEdit(fm: Frontmatter) {
    const reserved = ['tags', 'categories'];
    const edit: Record<string, string> = {};
    for (const [k, v] of Object.entries(fm)) {
      if (!reserved.includes(k)) {
        edit[k] = typeof v === 'boolean' ? String(v) : String(v || '');
      }
    }
    edit.title = edit.title || '';
    edit.date = edit.date || '';
    edit.draft = edit.draft || 'true';
    edit.cover = edit.cover || '';
    edit.description = edit.description || '';

    setFmEdit(edit);
    setFmTagsStr(Array.isArray(fm.tags) ? fm.tags.join(', ') : (fm.tags || '') as string);
    setFmCategoriesStr(Array.isArray(fm.categories) ? fm.categories.join(', ') : (fm.categories || '') as string);

    setFmDateLocal('');
    if (edit.date) {
      const d = new Date(edit.date);
      if (!isNaN(d.getTime())) {
        const pad = (n: number) => String(n).padStart(2, '0');
        setFmDateLocal(`${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`);
      }
    }

    const coreKeys = ['title', 'date', 'draft', 'tags', 'categories', 'cover', 'description'];
    const extra: Array<{ key: string; value: string }> = [];
    for (const [k, v] of Object.entries(fm)) {
      if (!coreKeys.includes(k)) {
        extra.push({ key: k, value: typeof v === 'object' ? JSON.stringify(v) : String(v) });
      }
    }
    setFmExtraFields(extra);
  }

  function applyFrontmatter() {
    const fm: Frontmatter = {};
    for (const [k, v] of Object.entries(fmEdit)) {
      if (v !== '' && v !== undefined && v !== null) {
        if (k === 'draft') {
          fm[k] = v === 'true';
        } else {
          fm[k] = v;
        }
      }
    }
    if (fmTagsStr.trim()) {
      fm.tags = fmTagsStr.split(',').map((s) => s.trim()).filter(Boolean);
    }
    if (fmCategoriesStr.trim()) {
      fm.categories = fmCategoriesStr.split(',').map((s) => s.trim()).filter(Boolean);
    }
    for (const field of fmExtraFields) {
      if (field.key.trim()) {
        fm[field.key.trim()] = field.value;
      }
    }
    setFrontmatter(fm);
    setShowFrontmatterDrawer(false);
  }

  function updatePreview() {
    let html = renderMarkdown(content || '');
    if (currentFile) {
      const articleDir = currentFile.replace(/[^/]+$/, '');
      html = html.replace(/<img\s+src="([^"]+)"/g, (_, src) => {
        if (!src.startsWith('http://') && !src.startsWith('https://') && !src.startsWith('/')) {
          src = `/content/${articleDir}${src}`;
        }
        return `<img src="${src}"`;
      });
    }
    const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    html = html.replace(/\{\{&lt;\s*ref\s+"([^"]+)"\s*&gt;\}\}/g, (_: string, path: string) => {
      const p = esc(path);
      return `<a href="/editor/${p}" target="_blank" class="inline-flex items-center px-2 py-0.5 rounded bg-blue-50 text-blue-700 hover:bg-blue-100 text-sm" title="点击编辑: ${p}">🔗 ${p}</a>`;
    });
    html = html.replace(/\{\{<\s*ref\s+"([^"]+)"\s*>\}\}/g, (_: string, path: string) => {
      const p = esc(path);
      return `<a href="/editor/${p}" target="_blank" class="inline-flex items-center px-2 py-0.5 rounded bg-blue-50 text-blue-700 hover:bg-blue-100 text-sm" title="点击编辑: ${p}">🔗 ${p}</a>`;
    });
    setPreview(html);
  }

  function insertMarkdown(type: string) {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const selectedText = content.substring(start, end);
    let insertion = '';
    let cursorOffset = 0;

    switch (type) {
      case 'bold':
        insertion = `**${selectedText || '粗体文本'}**`;
        cursorOffset = selectedText ? insertion.length : 2;
        break;
      case 'italic':
        insertion = `*${selectedText || '斜体文本'}*`;
        cursorOffset = selectedText ? insertion.length : 1;
        break;
      case 'heading':
        insertion = `## ${selectedText || '标题'}`;
        cursorOffset = selectedText ? insertion.length : 3;
        break;
      case 'link':
        insertion = `[${selectedText || '链接文本'}](url)`;
        cursorOffset = selectedText ? insertion.length - 4 : insertion.length - 5;
        break;
      case 'image':
        insertion = `![${selectedText || '图片描述'}](图片地址)`;
        cursorOffset = selectedText ? insertion.length - 6 : insertion.length - 11;
        break;
      case 'ul':
        insertion = `- ${selectedText || '列表项'}`;
        cursorOffset = insertion.length;
        break;
      case 'ol':
        insertion = `1. ${selectedText || '列表项'}`;
        cursorOffset = insertion.length;
        break;
      case 'quote':
        insertion = `> ${selectedText || '引用内容'}`;
        cursorOffset = insertion.length;
        break;
      case 'code':
        if (selectedText.includes('\n')) {
          insertion = `\`\`\`\n${selectedText || '代码'}\n\`\`\``;
          cursorOffset = selectedText ? insertion.length - 4 : 4;
        } else {
          insertion = `\`${selectedText || '代码'}\``;
          cursorOffset = selectedText ? insertion.length : 1;
        }
        break;
      case 'table':
        insertion = `| 列1 | 列2 | 列3 |\n| --- | --- | --- |\n| 内容 | 内容 | 内容 |`;
        cursorOffset = insertion.length;
        break;
      case 'ref':
        setShowRefModal(true);
        setRefSearchQuery('');
        setRefSearchResults([]);
        return;
    }

    const newContent = content.substring(0, start) + insertion + content.substring(end);
    setContent(newContent);
    requestAnimationFrame(() => {
      textarea.focus();
      textarea.setSelectionRange(start + cursorOffset, start + cursorOffset);
    });
  }

  async function loadFile() {
    setLoading(true);
    try {
      const data = await post<FileData & { success: boolean; message?: string }>('/api/file/read-with-frontmatter', { path: currentFile });
      if (data.success) {
        setContent(data.content || '');
        setOriginalContent(data.content || '');
        const fm = (data.frontmatter || {}) as Frontmatter;
        setFrontmatter(fm);
        setOriginalFrontmatter(JSON.parse(JSON.stringify(fm)));
        syncFmEdit(fm);
        await checkPublishStatus();
      } else {
        const fallback = await post<FileData & { success: boolean }>('/api/file/read', { path: currentFile });
        if (fallback.success) {
          setContent(fallback.content || '');
          setOriginalContent(fallback.content || '');
          setFrontmatter({});
          setOriginalFrontmatter({});
          await checkPublishStatus();
        } else {
          showNotification('加载文件失败', 'error');
        }
      }
    } catch (error) {
      showNotification('加载文件失败', 'error');
    } finally {
      setLoading(false);
    }
  }

  async function saveFile() {
    if (!currentFile) {
      showNotification('未选择文件', 'error');
      return;
    }
    setSaving(true);
    try {
      const body: Record<string, unknown> = { path: currentFile, content };
      if (Object.keys(frontmatter).length > 0) {
        body.frontmatter = frontmatter;
      }
      const data = await post<{ success: boolean; message?: string }>('/api/file/save', body);
      if (data.success) {
        setOriginalContent(content);
        setOriginalFrontmatter(JSON.parse(JSON.stringify(frontmatter)));
        showNotification('保存成功', 'success');
      } else {
        showNotification('保存失败: ' + data.message, 'error');
      }
    } catch (error) {
      showNotification('保存失败', 'error');
    } finally {
      setSaving(false);
    }
  }

  async function publishArticle() {
    if (!currentFile) {
      showNotification('未选择文件', 'error');
      return;
    }
    if (!confirm('确定要发布这篇文章吗？发布后将无法撤销草稿状态。')) return;
    setPublishing(true);
    try {
      const data = await post<{ success: boolean; message?: string }>('/api/article/publish', { path: currentFile });
      if (data.success) {
        showNotification('发布成功', 'success');
        setIsPublished(true);
        await loadFile();
      } else {
        showNotification('发布失败: ' + data.message, 'error');
      }
    } catch (error) {
      showNotification('发布失败', 'error');
    } finally {
      setPublishing(false);
    }
  }

  async function checkPublishStatus() {
    if (!currentFile) return;
    try {
      const data = await get<{ is_published: boolean }>(`/api/article/status?path=${encodeURIComponent(currentFile)}`);
      setIsPublished(data.is_published);
    } catch (error) {
      console.error('Failed to check publish status:', error);
    }
  }

  async function loadImages() {
    if (!currentFile) return;
    try {
      const data = await post<{ success: boolean; images?: ImageItem[] }>('/api/image/list', { path: currentFile });
      if (data.success) {
        setImages(data.images || []);
      }
    } catch (error) {
      console.error('Failed to load images:', error);
    }
  }

  async function uploadImage(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file || !currentFile) return;
    const formData = new FormData();
    formData.append('image', file);
    formData.append('path', currentFile);
    try {
      const response = await fetch('/api/image/upload', { method: 'POST', body: formData });
      const data = await response.json();
      if (data.success) {
        showNotification('图片上传成功', 'success');
        await loadImages();
      } else {
        showNotification('上传失败: ' + data.message, 'error');
      }
    } catch (error) {
      showNotification('上传失败', 'error');
    }
  }

  function copyImageUrl(url: string) {
    navigator.clipboard.writeText(url);
    showNotification('图片链接已复制', 'success');
  }

  async function loadBacklinks() {
    if (!currentFile) return;
    try {
      const data = await get<{ backlinks: Backlink[] }>(`/api/references/backlinks?path=${encodeURIComponent(currentFile)}`);
      setBacklinks(data.backlinks || []);
    } catch (error) {
      console.error('Failed to load backlinks:', error);
    }
  }

  async function searchRefs() {
    if (!refSearchQuery.trim()) {
      setRefSearchResults([]);
      return;
    }
    try {
      const data = await get<{ results: Array<{ path: string; title: string }> }>(`/api/posts/search?q=${encodeURIComponent(refSearchQuery)}`);
      setRefSearchResults(data.results || []);
    } catch (error) {
      console.error('Failed to search refs:', error);
    }
  }

  function insertRef(item: { path: string; title: string }) {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const insertion = `{{< ref "${item.path}" >}}`;
    const newContent = content.substring(0, start) + insertion + content.substring(end);
    setContent(newContent);
    setShowRefModal(false);
    requestAnimationFrame(() => {
      textarea.focus();
      textarea.setSelectionRange(start + insertion.length, start + insertion.length);
    });
  }

  function handlePaste(e: React.ClipboardEvent) {
    const items = e.clipboardData.items;
    for (let i = 0; i < items.length; i++) {
      if (items[i].type.indexOf('image') !== -1) {
        e.preventDefault();
        const blob = items[i].getAsFile();
        if (blob && currentFile) {
          const formData = new FormData();
          formData.append('image', blob);
          formData.append('path', currentFile);
          fetch('/api/image/upload', { method: 'POST', body: formData })
            .then((r) => r.json())
            .then((data) => {
              if (data.success) {
                const url = data.url || data.image_url;
                const textarea = textareaRef.current;
                if (textarea) {
                  const start = textarea.selectionStart;
                  const insertion = `![图片](${url})`;
                  const newContent = content.substring(0, start) + insertion + content.substring(start);
                  setContent(newContent);
                }
                loadImages();
              }
            })
            .catch(console.error);
        }
        return;
      }
    }
  }

  function showNotification(message: string, type: 'info' | 'success' | 'error' | 'warning' = 'info') {
    const notification = document.createElement('div');
    notification.className = `fixed top-4 right-4 px-6 py-3 rounded-lg shadow-lg z-50 fade-in ${
      type === 'success' ? 'bg-green-500' : type === 'error' ? 'bg-red-500' : type === 'warning' ? 'bg-yellow-500' : 'bg-blue-500'
    } text-white`;
    notification.textContent = message;
    document.body.appendChild(notification);
    setTimeout(() => notification.remove(), 3000);
  }

  if (!currentFile) {
    return (
      <div className="bg-white rounded-lg shadow p-12 text-center">
        <FileCode className="w-16 h-16 text-gray-400 mx-auto mb-4" />
        <h3 className="text-lg font-semibold text-gray-700 mb-2">未选择文件</h3>
        <p className="text-gray-500 mb-4">请从文章列表中选择一个文件进行编辑</p>
        <Link to="/posts" className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors">
          浏览文章
        </Link>
      </div>
    );
  }

  return (
    <div className="relative">
      {/* 工具栏 */}
      <div className="bg-white rounded-lg shadow p-4 mb-4">
        <div className="flex items-center justify-between mb-4 flex-wrap gap-2">
          <div className="flex items-center space-x-4">
            <div>
              <label className="text-sm font-medium text-gray-700">当前文件:</label>
              <span className="ml-2 text-gray-600 font-mono text-sm">{currentFile}</span>
            </div>
          </div>
          <div className="flex items-center space-x-2 flex-wrap">
            <button onClick={() => setShowFrontmatterDrawer(true)} className="px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 transition-colors flex items-center">
              <FileCode className="w-5 h-5 mr-2" />
              Frontmatter
            </button>
            <button onClick={() => setShowBacklinks(!showBacklinks)} className="px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 transition-colors flex items-center">
              <ArrowLeftRight className="w-5 h-5 mr-2" />
              反向链接
              {backlinks.length > 0 && (
                <span className="ml-1 bg-blue-100 text-blue-800 text-xs font-semibold px-2 py-0.5 rounded-full">{backlinks.length}</span>
              )}
            </button>
            <button onClick={() => setShowImageManager(!showImageManager)} className="px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50 transition-colors flex items-center">
              <Image className="w-5 h-5 mr-2" />
              图片管理
            </button>
            <button onClick={saveFile} disabled={saving || !hasChanges} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center">
              <Save className="w-5 h-5 mr-2" />
              {saving ? '保存中...' : hasChanges ? '保存 (Ctrl+S)' : '已保存'}
            </button>
            <button onClick={publishArticle} disabled={publishing || !currentFile} className={`px-4 py-2 text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center ${isPublished ? 'bg-gray-400 hover:bg-gray-500' : 'bg-green-600 hover:bg-green-700'}`}>
              <Upload className="w-5 h-5 mr-2" />
              {publishing ? '发布中...' : isPublished ? '已发布' : '发布'}
            </button>
          </div>
        </div>

        {showImageManager && (
          <div className="border-t pt-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="font-semibold text-gray-900">图片管理</h3>
              <label className="px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded-lg cursor-pointer transition-colors">
                <input type="file" onChange={uploadImage} accept="image/*" className="hidden" />
                上传图片
              </label>
            </div>
            <div className="grid grid-cols-4 gap-4">
              {images.map((image) => {
                const articleDir = currentFile.replace(/[^/]+$/, '');
                return (
                  <div key={image.name} className="relative group">
                    <img src={`/content/${articleDir}${image.url}`} alt={image.name} className="w-full h-32 object-cover rounded border border-gray-200" />
                    <div className="absolute inset-0 bg-black bg-opacity-50 opacity-0 group-hover:opacity-100 transition-opacity rounded flex items-center justify-center z-10">
                      <button onClick={() => copyImageUrl(image.url)} className="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded shadow-lg">
                        复制链接
                      </button>
                    </div>
                    <div className="mt-1 text-xs text-gray-600 truncate">{image.name}</div>
                  </div>
                );
              })}
            </div>
            {images.length === 0 && <p className="text-gray-500 text-center py-8">暂无图片，点击"上传图片"开始上传</p>}
          </div>
        )}
      </div>

      {/* Markdown 工具栏 */}
      <div className="bg-white rounded-lg shadow p-3 mb-4">
        <div className="flex flex-wrap gap-2">
          <button onClick={() => insertMarkdown('bold')} className="toolbar-btn" title="加粗 (Ctrl+B)">
            <Bold className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('italic')} className="toolbar-btn" title="斜体 (Ctrl+I)">
            <Italic className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('heading')} className="toolbar-btn" title="标题">
            <Heading className="w-4 h-4" />
          </button>
          <span className="border-l border-gray-300 mx-1" />
          <button onClick={() => insertMarkdown('link')} className="toolbar-btn" title="链接">
            <Link2 className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('ref')} className="toolbar-btn" title="文章引用">
            <ArrowLeftRight className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('image')} className="toolbar-btn" title="图片">
            <Image className="w-4 h-4" />
          </button>
          <span className="border-l border-gray-300 mx-1" />
          <button onClick={() => insertMarkdown('ul')} className="toolbar-btn" title="无序列表">
            <List className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('ol')} className="toolbar-btn" title="有序列表">
            <ListOrdered className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('quote')} className="toolbar-btn" title="引用">
            <Quote className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('code')} className="toolbar-btn" title="代码">
            <Code className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('table')} className="toolbar-btn" title="表格">
            <Table className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* 编辑器和预览 */}
      <div className="bg-white rounded-lg shadow p-4">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4" style={{ height: 'calc(100vh - 380px)', minHeight: '500px' }}>
          <div className="flex flex-col">
            <div className="text-sm font-medium text-gray-700 mb-2">编辑器</div>
            <textarea
              ref={textareaRef}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              onPaste={handlePaste}
              placeholder="在此输入 Markdown 内容..."
              className="flex-1 w-full font-mono text-sm p-4 border border-gray-200 rounded-lg resize-none focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
              style={{ lineHeight: 1.6 }}
            />
          </div>
          <div className="flex flex-col">
            <div className="text-sm font-medium text-gray-700 mb-2">预览</div>
            <div className="flex-1 overflow-y-auto p-4 border border-gray-200 rounded-lg bg-white">
              <div className="markdown-body" dangerouslySetInnerHTML={{ __html: preview }} />
            </div>
          </div>
        </div>
      </div>

      {/* Frontmatter 抽屉 */}
      {showFrontmatterDrawer && (
        <>
          <div className="fixed inset-0 bg-black bg-opacity-30 z-40" onClick={() => setShowFrontmatterDrawer(false)} />
          <div className="fixed top-0 right-0 w-[420px] max-w-[90vw] h-screen bg-white shadow-xl z-50 overflow-y-auto flex flex-col">
            <div className="flex items-center justify-between p-4 border-b">
              <h3 className="text-lg font-semibold text-gray-900">Frontmatter</h3>
              <button onClick={() => setShowFrontmatterDrawer(false)} className="text-gray-400 hover:text-gray-600">
                <X className="w-6 h-6" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">title</label>
                <input type="text" value={fmEdit.title || ''} onChange={(e) => setFmEdit({ ...fmEdit, title: e.target.value })} placeholder="文章标题" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">date</label>
                <input
                  type="datetime-local"
                  value={fmDateLocal}
                  onChange={(e) => {
                    setFmDateLocal(e.target.value);
                    setFmEdit({ ...fmEdit, date: e.target.value ? e.target.value.replace('T', ' ') + ':00+08:00' : '' });
                  }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">draft</label>
                <select value={fmEdit.draft || 'true'} onChange={(e) => setFmEdit({ ...fmEdit, draft: e.target.value })} className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500">
                  <option value="true">true (草稿)</option>
                  <option value="false">false (已发布)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">tags (逗号分隔)</label>
                <input type="text" value={fmTagsStr} onChange={(e) => setFmTagsStr(e.target.value)} placeholder="tag1, tag2" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">categories (逗号分隔)</label>
                <input type="text" value={fmCategoriesStr} onChange={(e) => setFmCategoriesStr(e.target.value)} placeholder="cat1, cat2" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">cover</label>
                <input type="text" value={fmEdit.cover || ''} onChange={(e) => setFmEdit({ ...fmEdit, cover: e.target.value })} placeholder="封面图片路径" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">description</label>
                <input type="text" value={fmEdit.description || ''} onChange={(e) => setFmEdit({ ...fmEdit, description: e.target.value })} placeholder="文章描述" className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500" />
              </div>
              <div className="border-t pt-4">
                <div className="flex items-center justify-between mb-2">
                  <label className="text-sm font-semibold text-gray-600">其他字段</label>
                  <button onClick={() => setFmExtraFields([...fmExtraFields, { key: '', value: '' }])} className="text-sm text-blue-600 hover:text-blue-700">+ 添加字段</button>
                </div>
                {fmExtraFields.map((field, i) => (
                  <div key={i} className="flex items-center space-x-2 mb-2">
                    <input type="text" value={field.key} onChange={(e) => {
                      const next = [...fmExtraFields];
                      next[i].key = e.target.value;
                      setFmExtraFields(next);
                    }} placeholder="key" className="w-1/3 px-2 py-1 border rounded text-sm" />
                    <input type="text" value={field.value} onChange={(e) => {
                      const next = [...fmExtraFields];
                      next[i].value = e.target.value;
                      setFmExtraFields(next);
                    }} placeholder="value" className="flex-1 px-2 py-1 border rounded text-sm" />
                    <button onClick={() => setFmExtraFields(fmExtraFields.filter((_, idx) => idx !== i))} className="text-red-400 hover:text-red-600">
                      <X className="w-4 h-4" />
                    </button>
                  </div>
                ))}
              </div>
            </div>
            <div className="p-4 border-t flex justify-end space-x-2">
              <button onClick={() => setShowFrontmatterDrawer(false)} className="px-4 py-2 border border-gray-300 rounded-lg text-gray-700 hover:bg-gray-50">取消</button>
              <button onClick={applyFrontmatter} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">应用</button>
            </div>
          </div>
        </>
      )}

      {/* 引用搜索弹窗 */}
      {showRefModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-40" onClick={() => setShowRefModal(false)}>
          <div className="bg-white rounded-xl shadow-2xl w-full max-w-lg mx-4 overflow-hidden" onClick={(e) => e.stopPropagation()}>
            <div className="p-4 border-b">
              <h3 className="text-lg font-semibold mb-3">搜索文章引用</h3>
              <input
                type="text"
                value={refSearchQuery}
                onChange={(e) => { setRefSearchQuery(e.target.value); searchRefs(); }}
                placeholder="输入关键词搜索文章..."
                className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 outline-none"
                autoFocus
              />
            </div>
            <div className="max-h-80 overflow-y-auto">
              {refSearchResults.map((item) => (
                <div key={item.path} onClick={() => insertRef(item)} className="px-4 py-3 hover:bg-blue-50 cursor-pointer border-b border-gray-100 flex items-center gap-3">
                  <span className="text-blue-500 text-lg">🔗</span>
                  <div className="min-w-0">
                    <div className="font-medium text-gray-800 truncate">{item.title || '(无标题)'}</div>
                    <div className="text-sm text-gray-400 truncate">{item.path}</div>
                  </div>
                </div>
              ))}
              {refSearchQuery && refSearchResults.length === 0 && (
                <div className="p-6 text-center text-gray-400">未找到匹配的文章</div>
              )}
              {!refSearchQuery && (
                <div className="p-6 text-center text-gray-400">输入关键词开始搜索</div>
              )}
            </div>
            <div className="p-3 border-t bg-gray-50 flex justify-end">
              <button onClick={() => setShowRefModal(false)} className="px-4 py-2 text-gray-600 hover:text-gray-800">取消</button>
            </div>
          </div>
        </div>
      )}

      {/* 反向链接面板 */}
      {showBacklinks && (
        <>
          <div className="fixed inset-0 bg-black bg-opacity-30 z-40" onClick={() => setShowBacklinks(false)} />
          <div className="fixed top-0 right-0 w-[380px] max-w-[90vw] h-screen bg-white shadow-xl z-50 overflow-y-auto flex flex-col">
            <div className="flex items-center justify-between p-4 border-b">
              <h3 className="text-lg font-semibold text-gray-900">反向链接</h3>
              <button onClick={() => setShowBacklinks(false)} className="text-gray-400 hover:text-gray-600">
                <X className="w-6 h-6" />
              </button>
            </div>
            {backlinks.length === 0 && <div className="p-4 text-sm text-gray-500">暂无其他文章引用本文</div>}
            {backlinks.map((bl) => (
              <Link key={bl.path} to={`/editor/${bl.path}`} target="_blank" className="block p-4 border-b border-gray-100 hover:bg-gray-50 transition-colors">
                <div className="font-medium text-gray-900">{bl.title || '(无标题)'}</div>
                <div className="text-xs text-gray-400">{bl.path}</div>
                {bl.context && <div className="text-xs text-gray-500 bg-gray-100 p-2 rounded mt-2">{bl.context}</div>}
              </Link>
            ))}
          </div>
        </>
      )}

      {/* 加载状态 */}
      {loading && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-8 text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mb-4" />
            <p className="text-gray-700">加载中...</p>
          </div>
        </div>
      )}
    </div>
  );
}
