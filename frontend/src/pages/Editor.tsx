import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
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
  Sparkles,
  Wand2,
  Headphones,
} from 'lucide-react';
import { get, post } from '../utils/api';
import {
  generateArticleTTS,
  deleteArticleTTS,
  getTTSStatus,
} from '../utils/api';
import { renderMarkdown, escapeHtml } from '../utils/markdown';
import type { Mermaid } from 'mermaid';
import type { FileData, ImageItem, Backlink, Frontmatter } from '../types';
import { usePageTitle } from '../hooks/usePageTitle';
import { useSocket } from '../hooks/useSocket';
import { InlineEditOverlay } from '../components/InlineEdit/Overlay';
import { ConflictModal } from '../components/ConflictModal';

// Mermaid is heavy (~800KB); load it lazily and only when the preview
// actually contains a ```mermaid block. Initialised once per session.
let mermaidPromise: Promise<Mermaid> | null = null;
function loadMermaid(): Promise<Mermaid> {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid')
      .then((mod) => {
        const api = mod.default;
        api.initialize({
          startOnLoad: false,
          theme: 'default',
          // strict: mermaid runs after DOMPurify on the markdown body, so we
          // must not let diagram source inject arbitrary HTML/links. Mermaid
          // escapes/encodes under strict mode; 'loose' would bypass sanitiser.
          securityLevel: 'strict',
          fontFamily: 'inherit',
        });
        return api;
      })
      .catch((error) => {
        // Clear the cache so a transient chunk-load failure (offline, deploy
        // in progress) can be retried on the next preview update instead of
        // sticking the session on a rejected promise.
        mermaidPromise = null;
        throw error;
      });
  }
  return mermaidPromise;
}

export default function Editor() {
  const { filePath, '*': restPath } = useParams();
  // navigate not used in this page
  const fullPath = restPath ? `${filePath}/${restPath}` : filePath || '';

  const [content, setContent] = useState('');
  const [originalContent, setOriginalContent] = useState('');
  const [preview, setPreview] = useState('');

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
  const hasChanges = useMemo(
    () => content !== originalContent || JSON.stringify(frontmatter) !== JSON.stringify(originalFrontmatter),
    [content, originalContent, frontmatter, originalFrontmatter],
  );
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
  const previewRef = useRef<HTMLDivElement>(null);
  const saveFileRef = useRef<(force?: boolean) => Promise<void>>(async () => {});
  const insertMarkdownRef = useRef<(type: string) => void>(() => {});
  const [generatingCover, setGeneratingCover] = useState(false);
  const [generatingFm, setGeneratingFm] = useState(false);
  const [fileMtime, setFileMtime] = useState<number | null>(null);
  const [conflictRemoteContent, setConflictRemoteContent] = useState<string | null>(null);
  const { setTitle: setPageTitle, resetTitle: resetPageTitle } = usePageTitle();

  // TTS 语音播报
  const socketRef = useSocket();
  const [ttsAvailable, setTtsAvailable] = useState(false);
  const [ttsVoices, setTtsVoices] = useState<string[]>([]);
  const [generatingTts, setGeneratingTts] = useState(false);
  const [ttsProgress, setTtsProgress] = useState<{ stage: string; percent: number; message: string } | null>(null);
  const [ttsVoice, setTtsVoice] = useState('');
  const [ttsSpeed, setTtsSpeed] = useState(1.0);
  const [audioUrl, setAudioUrl] = useState('');

  useEffect(() => {
    if (frontmatter.title) {
      setPageTitle(frontmatter.title);
    }
    return () => { resetPageTitle(); };
  }, [frontmatter.title, setPageTitle, resetPageTitle]);

  const currentFile = fullPath;

  const updatePreview = useCallback(() => {
    let html = renderMarkdown(content || '', { mermaid: true });
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
  }, [content, currentFile]);

  const loadBacklinks = useCallback(async () => {
    if (!currentFile) return;
    try {
      const data = await get<{ backlinks: Backlink[] }>(`/api/references/backlinks?path=${encodeURIComponent(currentFile)}`);
      setBacklinks(data.backlinks || []);
    } catch (error) {
      console.error('Failed to load backlinks:', error);
    }
  }, [currentFile]);

  const loadImages = useCallback(async () => {
    if (!currentFile) return;
    try {
      const data = await post<{ success: boolean; images?: ImageItem[] }>('/api/image/list', { article_path: currentFile });
      if (data.success) {
        setImages(data.images || []);
      }
    } catch (error) {
      console.error('Failed to load images:', error);
    }
  }, [currentFile]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && showRefModal) {
        setShowRefModal(false);
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasChanges && !saving) saveFileRef.current();
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'b') {
        e.preventDefault();
        insertMarkdownRef.current('bold');
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'i') {
        e.preventDefault();
        insertMarkdownRef.current('italic');
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
    edit.image = edit.image || '';
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

    const coreKeys = ['title', 'date', 'draft', 'tags', 'categories', 'cover', 'image', 'description'];
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

  const insertMarkdown = useCallback((type: string) => {
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
  }, [content]);

  function applyInlineEdit(revisedText: string, anchorStart: number, anchorEnd: number) {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const newContent =
      content.substring(0, anchorStart) + revisedText + content.substring(anchorEnd);
    setContent(newContent);
    requestAnimationFrame(() => {
      textarea.focus();
      textarea.setSelectionRange(
        anchorStart,
        anchorStart + revisedText.length,
      );
    });
  }

  function handleInlineEditDrift() {
    showNotification('选区已变化，已取消', 'warning');
  }

  async function discardAndReload() {
    setConflictRemoteContent(null);
    await loadFile();
    showNotification('已加载最新版本', 'info');
  }

  const saveFile = useCallback(async (force = false) => {
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
      if (!force && fileMtime !== null) {
        body.expected_mtime = fileMtime;
      }
      if (force) {
        body.force = true;
      }
      const res = await fetch('/api/file/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const data = await res.json();
      if (res.status === 409 && data.conflict) {
        setConflictRemoteContent(data.current_content);
        return;
      }
      if (data.success) {
        setOriginalContent(content);
        setOriginalFrontmatter(JSON.parse(JSON.stringify(frontmatter)));
        setFileMtime(data.mtime ?? null);
        setConflictRemoteContent(null);
        showNotification('保存成功', 'success');
      } else {
        showNotification('保存失败: ' + data.message, 'error');
      }
    } catch {
      showNotification('保存失败', 'error');
    } finally {
      setSaving(false);
    }
  }, [currentFile, content, frontmatter, fileMtime]);

  async function publishArticle() {
    if (!currentFile) {
      showNotification('未选择文件', 'error');
      return;
    }
    if (isPublished) {
      showNotification('文章已发布', 'info');
      return;
    }
    if (!confirm('确定要发布这篇文章吗？发布后将无法撤销草稿状态。')) return;
    setPublishing(true);
    try {
      const data = await post<{ success: boolean; message?: string }>('/api/article/publish', { file_path: currentFile });
      if (data.success) {
        showNotification('发布成功', 'success');
        setIsPublished(true);
        await loadFile();
      } else {
        showNotification('发布失败: ' + data.message, 'error');
      }
    } catch {
      showNotification('发布失败', 'error');
    } finally {
      setPublishing(false);
    }
  }

  const checkPublishStatus = useCallback(async () => {
    if (!currentFile) return;
    try {
      const data = await get<{ status: { is_draft?: boolean } }>(`/api/article/status?file_path=${encodeURIComponent(currentFile)}`);
      setIsPublished(!data.status?.is_draft);
    } catch (error) {
      console.error('Failed to check publish status:', error);
    }
  }, [currentFile]);

  async function uploadImage(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file || !currentFile) return;
    const formData = new FormData();
    formData.append('file', file);
    formData.append('article_path', currentFile);
    try {
      const response = await fetch('/api/image/upload', { method: 'POST', body: formData });
      const data = await response.json();
      if (data.success) {
        showNotification('图片上传成功', 'success');
        await loadImages();
      } else {
        showNotification('上传失败: ' + data.message, 'error');
      }
    } catch {
      showNotification('上传失败', 'error');
    }
  }

  async function generateCoverImage() {
    if (!currentFile) {
      showNotification('未选择文件', 'error');
      return;
    }
    setGeneratingCover(true);
    try {
      const data = await post<{ success: boolean; url?: string; message?: string }>('/api/image/generate-cover', {
        article_path: currentFile,
        title: fmEdit.title || frontmatter.title || '',
        description: fmEdit.description || frontmatter.description || '',
        content,
      });
      if (data.success && data.url) {
        setFmEdit({ ...fmEdit, cover: data.url, image: data.url });
        setFrontmatter({ ...frontmatter, cover: data.url, image: data.url });
        showNotification('封面图片已生成', 'success');
        await loadImages();
      } else {
        showNotification('生成失败: ' + (data.message || '未知错误'), 'error');
      }
    } catch {
      showNotification('生成封面失败', 'error');
    } finally {
      setGeneratingCover(false);
    }
  }

  async function generateSpeech() {
    if (!currentFile) {
      showNotification('未选择文件', 'error');
      return;
    }
    if (!content.trim()) {
      showNotification('文章内容为空', 'error');
      return;
    }
    setGeneratingTts(true);
    setTtsProgress({ stage: 'starting', percent: 0, message: '' });
    try {
      const data = await generateArticleTTS(currentFile, {
        voice: ttsVoice || undefined,
        speed: ttsSpeed,
      });
      if (!data.success) {
        setGeneratingTts(false);
        setTtsProgress(null);
        showNotification('语音生成失败: ' + (data.message || '未知错误'), 'error');
      }
      // pending=true 时进度与结果由 Socket.IO 事件驱动
    } catch {
      setGeneratingTts(false);
      setTtsProgress(null);
      showNotification('语音生成请求失败', 'error');
    }
  }

  async function deleteSpeech() {
    if (!currentFile) return;
    if (!confirm('确定删除该文章的语音播报？')) return;
    try {
      const data = await deleteArticleTTS(currentFile);
      if (data.success) {
        setAudioUrl('');
        setFrontmatter((prev) => {
          const next = { ...prev };
          delete next.audio;
          delete next.audio_duration_seconds;
          delete next.audio_format;
          return next;
        });
        setFmEdit((prev) => {
          const next = { ...prev };
          delete next.audio;
          return next;
        });
        // 后端已保存文件，刷新 fileMtime 避免下次手动保存触发假冲突
        if (data.mtime) setFileMtime(data.mtime);
        showNotification('已删除语音', 'success');
      } else {
        showNotification('删除失败: ' + (data.message || '未知错误'), 'error');
      }
    } catch {
      showNotification('删除语音失败', 'error');
    }
  }

  async function generateFrontmatterFromAI() {
    if (!currentFile || !content.trim()) {
      showNotification('文章内容为空', 'error');
      return;
    }
    setGeneratingFm(true);
    try {
      const data = await post<{ success: boolean; frontmatter?: Record<string, unknown>; message?: string }>('/api/frontmatter/generate', { content });
      if (data.success && data.frontmatter) {
        const fm = data.frontmatter;
        const newFmEdit = { ...fmEdit };
        if (fm.description && typeof fm.description === 'string') newFmEdit.description = fm.description;
        setFmEdit(newFmEdit);
        if (Array.isArray(fm.tags)) setFmTagsStr(fm.tags.join(', '));
        if (Array.isArray(fm.categories)) setFmCategoriesStr(fm.categories.join(', '));
        const newFm: Frontmatter = { ...frontmatter };
        if (typeof fm.description === 'string') newFm.description = fm.description;
        if (Array.isArray(fm.tags)) newFm.tags = fm.tags as string[];
        if (Array.isArray(fm.categories)) newFm.categories = fm.categories as string[];
        setFrontmatter(newFm);
        showNotification('Frontmatter 已生成，点击保存生效', 'success');
      } else {
        showNotification('生成失败: ' + (data.message || '未知错误'), 'error');
      }
    } catch (error) {
      console.error('Failed to generate frontmatter:', error);
      showNotification('生成 frontmatter 失败', 'error');
    } finally {
      setGeneratingFm(false);
    }
  }

  function copyImageUrl(url: string) {
    navigator.clipboard.writeText(url);
    showNotification('图片链接已复制', 'success');
  }

  async function searchRefs() {
    if (!refSearchQuery.trim()) {
      setRefSearchResults([]);
      return;
    }
    try {
      const data = await get<{ posts: Array<{ path: string; title: string }> }>(`/api/posts/search?q=${encodeURIComponent(refSearchQuery)}`);
      setRefSearchResults(data.posts || []);
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
          const cursorPos = textareaRef.current?.selectionStart ?? -1;
          const formData = new FormData();
          formData.append('file', blob);
          formData.append('article_path', currentFile);
          fetch('/api/image/upload', { method: 'POST', body: formData })
            .then((r) => r.json())
            .then((data) => {
              if (data.success) {
                const url = data.url || data.image_url;
                const insertion = `![图片](${url})`;
                setContent((prev) => {
                  const pos = cursorPos >= 0 ? cursorPos : prev.length;
                  return prev.substring(0, pos) + insertion + prev.substring(pos);
                });
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

  const loadFile = useCallback(async () => {
    if (!currentFile) return;
    setLoading(true);
    try {
      const data = await post<FileData & { success: boolean; message?: string; mtime?: number }>('/api/file/read-with-frontmatter', { path: currentFile });
      if (data.success) {
        setContent(data.content || '');
        setOriginalContent(data.content || '');
        setFileMtime(data.mtime ?? null);
        const fm = (data.frontmatter || {}) as Frontmatter;
        setFrontmatter(fm);
        setOriginalFrontmatter(JSON.parse(JSON.stringify(fm)));
        syncFmEdit(fm);
        await checkPublishStatus();
      } else {
        const fallback = await post<FileData & { success: boolean; mtime?: number }>('/api/file/read', { path: currentFile });
        if (fallback.success) {
          setContent(fallback.content || '');
          setOriginalContent(fallback.content || '');
          setFileMtime(fallback.mtime ?? null);
          setFrontmatter({});
          setOriginalFrontmatter({});
          await checkPublishStatus();
        } else {
          showNotification('加载文件失败', 'error');
        }
      }
    } catch {
      showNotification('加载文件失败', 'error');
    } finally {
      setLoading(false);
    }
  }, [currentFile, checkPublishStatus]);

  // Keep callback refs current without writing during render.
  useEffect(() => {
    saveFileRef.current = saveFile;
    insertMarkdownRef.current = insertMarkdown;
  }, [saveFile, insertMarkdown]);

  useEffect(() => {
    (async () => {
      if (currentFile) {
        await loadFile();
        await loadImages();
        await loadBacklinks();
      }
    })();
  }, [currentFile, loadFile, loadImages, loadBacklinks]);

  // 同步 frontmatter.audio 到 audioUrl 状态（loadFile/保存后随之更新）
  useEffect(() => {
    const a = frontmatter.audio;
    setAudioUrl(typeof a === 'string' ? a : '');
  }, [frontmatter.audio]);

  // 查询 TTS 能力是否可用（控制按钮显隐）
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const st = await getTTSStatus();
        if (cancelled) return;
        setTtsAvailable(st.available);
        setTtsVoices(st.voices || []);
      } catch {
        if (!cancelled) setTtsAvailable(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  // 监听 TTS Socket.IO 进度/结果事件
  useEffect(() => {
    const socket = socketRef.current;
    if (!socket) return;
    const onProgress = (d: { stage?: string; percent?: number; message?: string }) => {
      setTtsProgress({ stage: d.stage || '', percent: d.percent ?? 0, message: d.message || '' });
    };
    const onDone = (d: { url?: string; duration_seconds?: number; format?: string; mtime?: number }) => {
      setGeneratingTts(false);
      setTtsProgress(null);
      if (d.url) {
        setAudioUrl(d.url);
        // 持久化后端已写入的全部音频字段，避免下次保存把它们丢掉
        setFrontmatter((prev) => ({
          ...prev,
          audio: d.url,
          audio_duration_seconds: d.duration_seconds,
          audio_format: d.format,
        }));
        setFmEdit((prev) => ({
          ...prev,
          audio: d.url,
          audio_duration_seconds: d.duration_seconds ? String(d.duration_seconds) : '',
          audio_format: d.format,
        }));
        // 后端已保存文件，刷新 fileMtime 避免下次手动保存触发假冲突
        if (d.mtime) setFileMtime(d.mtime);
        showNotification('语音播报已生成', 'success');
      }
    };
    const onFailed = (d: { message?: string }) => {
      setGeneratingTts(false);
      setTtsProgress(null);
      showNotification('语音生成失败: ' + (d.message || '未知错误'), 'error');
    };
    const onConflict = (d: { message?: string }) => {
      setGeneratingTts(false);
      setTtsProgress(null);
      showNotification(d.message || '文章已被修改，请保存后重试', 'warning');
    };
    socket.on('tts.progress', onProgress);
    socket.on('tts.done', onDone);
    socket.on('tts.failed', onFailed);
    socket.on('tts.conflict', onConflict);
    return () => {
      socket.off('tts.progress', onProgress);
      socket.off('tts.done', onDone);
      socket.off('tts.failed', onFailed);
      socket.off('tts.conflict', onConflict);
    };
  }, [socketRef]);

  useEffect(() => {
    (async () => {
      await updatePreview();
    })();
  }, [content, currentFile, updatePreview]);

  // Render ```mermaid blocks inside the preview pane. Debounced because the
  // effect fires on every keystroke (the whole `preview` HTML string changes)
  // and mermaid.run() is not cancellable mid-flight; without debouncing, rapid
  // typing would queue redundant renders that land on detached DOM nodes.
  //
  // Two robustness measures, both learned the hard way:
  //   1. Parse errors are caught via `api.parse()` and rendered as a visible
  //      error <pre>. Relying on mermaid.run()'s built-in error graphic is
  //      unreliable — its DOM-mutation-on-throw depends on singleton state.
  //   2. The preview div is memoised (see previewEl below) so unrelated state
  //      changes (images, backlinks, loading) don't make React reconcile
  //      dangerouslySetInnerHTML and detach the node mermaid just mutated —
  //      which would silently discard the rendered diagram or error block.
  useEffect(() => {
    const root = previewRef.current;
    if (!root) return;
    const nodes = Array.from(root.querySelectorAll<HTMLDivElement>('.mermaid'));
    if (nodes.length === 0) return;
    let cancelled = false;
    const timer = window.setTimeout(() => {
      loadMermaid()
        .then(async (api) => {
          if (cancelled) return;
          await Promise.all(nodes.map(async (node) => {
            const code = node.textContent ?? '';
            node.removeAttribute('data-processed');
            try {
              await api.parse(code);
            } catch (err) {
              const msg = err instanceof Error ? err.message : String(err);
              node.innerHTML =
                `<pre class="mermaid-error" style="margin:0;padding:8px;border:1px solid #fca5a5;background:#fef2f2;color:#991b1b;border-radius:6px;font-family:Monaco,Menlo,Consolas,monospace;font-size:0.85em;white-space:pre-wrap;word-break:break-word">Mermaid 语法错误:\n${escapeHtml(msg)}</pre>`;
              return;
            }
            try {
              await api.run({ nodes: [node] });
            } catch {
              // parse() passed but run() still failed (e.g. layout error);
              // rare. Leave the raw source rather than masking.
            }
          }));
        })
        .catch(() => {
          // Mermaid failed to load (e.g. offline); leave the raw diagram text.
        });
    }, 200);
    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [preview]);
  // Memoise the preview element so unrelated Editor state changes (images,
  // backlinks, loading, frontmatter drawer, …) do NOT recreate the React
  // element. When the element reference is stable, React bails out of
  // reconciling this subtree and leaves the mermaid-mutated DOM intact.
  // Without this, any sibling state change makes React re-apply
  // dangerouslySetInnerHTML and detach the .mermaid node mermaid just drew
  // into — the diagram (or its error block) vanishes, leaving raw source.
  const previewEl = useMemo(
    () => <div ref={previewRef} className="markdown-body" dangerouslySetInnerHTML={{ __html: preview }} />,
    [preview],
  );

  if (!currentFile) {
    return (
      <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-12 text-center">
        <FileCode className="w-16 h-16 text-stone-400 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-stone-700 mb-2">未选择文件</h3>
        <p className="text-stone-500 mb-4">请从文章列表中选择一个文件进行编辑</p>
        <Link to="/posts" className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors">
          浏览文章
        </Link>
      </div>
    );
  }

  return (
    <div className="relative">
      {/* 工具栏 */}
      <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-4 mb-4">
        <div className="flex items-center justify-between mb-4 flex-wrap gap-2">
          <div className="flex items-center space-x-4">
            <div>
              <label className="text-sm font-medium text-stone-700">当前文件:</label>
              <span className="ml-2 text-stone-600 font-mono text-sm">{currentFile}</span>
            </div>
          </div>
          <div className="flex items-center space-x-1">
            <button onClick={() => setShowFrontmatterDrawer(true)} title="Frontmatter" className="p-2 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors">
              <FileCode className="w-5 h-5" />
            </button>
            <button onClick={() => setShowBacklinks(!showBacklinks)} title="反向链接" className="relative p-2 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors">
              <ArrowLeftRight className="w-5 h-5" />
              {backlinks.length > 0 && (
                <span className="absolute -top-1 -right-1 bg-blue-100 text-blue-800 text-[10px] font-semibold w-4 h-4 flex items-center justify-center rounded-full">{backlinks.length}</span>
              )}
            </button>
            <button onClick={() => setShowImageManager(!showImageManager)} title="图片管理" className="p-2 border border-stone-300 text-stone-700 rounded-lg hover:bg-stone-50 transition-colors">
              <Image className="w-5 h-5" />
            </button>
            <button onClick={generateCoverImage} disabled={generatingCover || !currentFile} title={generatingCover ? '生成中...' : 'AI 生成封面'} className="p-2 border border-purple-400 text-purple-700 rounded-lg hover:bg-purple-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
              <Sparkles className="w-5 h-5" />
            </button>
            <button onClick={generateFrontmatterFromAI} disabled={generatingFm || !currentFile} title={generatingFm ? '生成中...' : 'AI 生成 Frontmatter'} className="p-2 border border-amber-400 text-amber-700 rounded-lg hover:bg-amber-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
              <Wand2 className="w-5 h-5" />
            </button>
            {ttsAvailable && (
              <button onClick={generateSpeech} disabled={generatingTts || !currentFile} title={generatingTts ? '生成语音中...' : '生成语音播报'} className="p-2 border border-emerald-400 text-emerald-700 rounded-lg hover:bg-emerald-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                <Headphones className="w-5 h-5" />
              </button>
            )}
            <span className="border-l border-stone-300 mx-1 h-6" />
            <button onClick={() => saveFile()} disabled={saving || !hasChanges} title={saving ? '保存中...' : hasChanges ? '保存 (Ctrl+S)' : '已保存'} className="p-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
              <Save className="w-5 h-5" />
            </button>
            <button onClick={publishArticle} disabled={publishing || !currentFile || isPublished} title={publishing ? '发布中...' : isPublished ? '已发布' : '发布'} className={`p-2 text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${isPublished ? 'bg-stone-400 hover:bg-stone-500' : 'bg-green-600 hover:bg-green-700'}`}>
              <Upload className="w-5 h-5" />
            </button>
          </div>
        </div>

        {(generatingCover || generatingFm) && (
          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-3 mb-4">
            <div className="flex items-center gap-3">
              <div className="w-full bg-stone-200 rounded-full h-2 overflow-hidden">
                <div className={`h-2 rounded-full animate-progress ${generatingCover ? 'bg-purple-500' : 'bg-amber-500'}`} style={{ width: '100%' }} />
              </div>
              <span className="text-sm text-stone-600 whitespace-nowrap">{generatingCover ? 'AI 生成封面中...' : 'AI 生成 Frontmatter 中...'}</span>
            </div>
          </div>
        )}

        {/* TTS 语音播报参数面板 + 进度 */}
        {ttsAvailable && (
          <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-3 mb-4">
            <div className="flex items-center gap-3 flex-wrap">
              <span className="text-sm font-medium text-stone-700">语音播报</span>
              {ttsVoices.length > 0 && (
                <select value={ttsVoice} onChange={(e) => setTtsVoice(e.target.value)} className="px-2 py-1 border border-stone-300 rounded-lg text-sm" disabled={generatingTts}>
                  {ttsVoices.map((v) => (<option key={v} value={v}>{v}</option>))}
                </select>
              )}
              <label className="flex items-center gap-2 text-sm text-stone-600">
                语速
                <input type="range" min={0.5} max={2} step={0.1} value={ttsSpeed} onChange={(e) => setTtsSpeed(parseFloat(e.target.value))} disabled={generatingTts} className="w-28" />
                <span className="font-mono w-8">{ttsSpeed.toFixed(1)}</span>
              </label>
              <button onClick={generateSpeech} disabled={generatingTts || !currentFile} className="px-3 py-1 bg-emerald-600 hover:bg-emerald-700 text-white rounded-lg text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                {audioUrl ? '重新生成' : '生成语音'}
              </button>
              {audioUrl && !generatingTts && (
                <button onClick={deleteSpeech} className="px-3 py-1 border border-stone-300 text-stone-600 rounded-lg text-sm hover:bg-stone-50 transition-colors">删除</button>
              )}
            </div>
            {generatingTts && (
              <div className="flex items-center gap-3 mt-3">
                <div className="w-full bg-stone-200 rounded-full h-2 overflow-hidden">
                  <div className="h-2 rounded-full bg-emerald-500 transition-all" style={{ width: `${Math.min(100, ttsProgress?.percent || 0)}%` }} />
                </div>
                <span className="text-sm text-stone-600 whitespace-nowrap">{ttsProgress?.message || ttsProgress?.stage || '生成中...'}</span>
              </div>
            )}
            {audioUrl && !generatingTts && (
              <audio controls src={audioUrl} className="w-full mt-3" />
            )}
          </div>
        )}

        {showImageManager && (
          <div className="border-t pt-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="font-semibold text-stone-900">图片管理</h3>
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
                    <img src={`/content/${articleDir}${image.url}`} alt={image.name} className="w-full h-32 object-cover rounded border border-stone-200" />
                    <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity rounded flex items-center justify-center z-10">
                      <button onClick={() => copyImageUrl(image.url)} className="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded shadow-lg">
                        复制链接
                      </button>
                    </div>
                    <div className="mt-1 text-xs text-stone-600 truncate">{image.name}</div>
                  </div>
                );
              })}
            </div>
            {images.length === 0 && <p className="text-stone-500 text-center py-8">暂无图片，点击"上传图片"开始上传</p>}
          </div>
        )}
      </div>

      {/* Markdown 工具栏 */}
      <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-3 mb-4">
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
          <span className="border-l border-stone-300 mx-1" />
          <button onClick={() => insertMarkdown('link')} className="toolbar-btn" title="链接">
            <Link2 className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('ref')} className="toolbar-btn" title="文章引用">
            <ArrowLeftRight className="w-4 h-4" />
          </button>
          <button onClick={() => insertMarkdown('image')} className="toolbar-btn" title="图片">
            <Image className="w-4 h-4" />
          </button>
          <span className="border-l border-stone-300 mx-1" />
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
      <div className="bg-white rounded-md ring-1 ring-stone-900/5 p-4">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4" style={{ height: 'calc(100vh - 380px)', minHeight: '500px' }}>
          <div className="flex flex-col">
            <div className="text-sm font-medium text-stone-700 mb-2">编辑器</div>
            <textarea
              ref={textareaRef}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              onPaste={handlePaste}
              placeholder="在此输入 Markdown 内容..."
              className="flex-1 w-full font-mono text-sm p-4 border border-stone-200 rounded-lg bg-white resize-none focus:ring-2 focus:ring-stone-400 focus:border-transparent outline-none"
              style={{ lineHeight: 1.6 }}
            />
            <InlineEditOverlay
              textareaRef={textareaRef}
              content={content}
              onAccept={applyInlineEdit}
              onDrift={handleInlineEditDrift}
            />
          </div>
          <div className="flex flex-col">
            <div className="text-sm font-medium text-stone-700 mb-2">预览</div>
            <div className="flex-1 overflow-y-auto p-4 border border-stone-200 rounded-lg bg-white">
              {previewEl}
            </div>
          </div>
        </div>
      </div>

      {/* Frontmatter 抽屉 */}
      {showFrontmatterDrawer && (
        <>
          <div className="fixed inset-0 bg-black/30 z-[55]" onClick={() => setShowFrontmatterDrawer(false)} />
          <div className="fixed top-0 right-0 w-[420px] max-w-[90vw] h-screen bg-white shadow-xl z-[60] overflow-y-auto flex flex-col">
            <div className="flex items-center justify-between p-4 border-b">
              <h3 className="text-lg font-medium text-stone-900">Frontmatter</h3>
              <button onClick={() => setShowFrontmatterDrawer(false)} className="text-stone-400 hover:text-stone-600">
                <X className="w-6 h-6" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              {(fmEdit.cover || fmEdit.image) && (
                <div className="relative rounded-lg overflow-hidden border border-stone-200">
                  <img
                    src={`/content/${currentFile.replace(/[^/]+$/, '')}${fmEdit.image || fmEdit.cover}`}
                    alt="封面图片"
                    className="w-full h-40 object-cover"
                  />
                  <button
                    onClick={() => {
                      const url = fmEdit.image || fmEdit.cover || '';
                      navigator.clipboard.writeText(url);
                    }}
                    className="absolute bottom-2 right-2 px-2 py-1 bg-black/60 text-white text-xs rounded hover:bg-black/80 transition-colors"
                  >
                    复制路径
                  </button>
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">title</label>
                <input type="text" value={fmEdit.title || ''} onChange={(e) => setFmEdit({ ...fmEdit, title: e.target.value })} placeholder="文章标题" className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400" />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">date</label>
                <input
                  type="datetime-local"
                  value={fmDateLocal}
                  onChange={(e) => {
                    setFmDateLocal(e.target.value);
                    setFmEdit({ ...fmEdit, date: e.target.value ? e.target.value.replace('T', ' ') + ':00+08:00' : '' });
                  }}
                  className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">draft</label>
                <select value={fmEdit.draft || 'true'} onChange={(e) => setFmEdit({ ...fmEdit, draft: e.target.value })} className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400">
                  <option value="true">true (草稿)</option>
                  <option value="false">false (已发布)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">tags (逗号分隔)</label>
                <input type="text" value={fmTagsStr} onChange={(e) => setFmTagsStr(e.target.value)} placeholder="tag1, tag2" className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400" />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">categories (逗号分隔)</label>
                <input type="text" value={fmCategoriesStr} onChange={(e) => setFmCategoriesStr(e.target.value)} placeholder="cat1, cat2" className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400" />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">cover</label>
                <input type="text" value={fmEdit.cover || ''} onChange={(e) => setFmEdit({ ...fmEdit, cover: e.target.value })} placeholder="封面图片路径" className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400" />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">image</label>
                <input type="text" value={fmEdit.image || ''} onChange={(e) => setFmEdit({ ...fmEdit, image: e.target.value })} placeholder="封面图片路径 (Hugo theme)" className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400" />
              </div>
              <div>
                <label className="block text-sm font-medium text-stone-700 mb-1">description</label>
                <input type="text" value={fmEdit.description || ''} onChange={(e) => setFmEdit({ ...fmEdit, description: e.target.value })} placeholder="文章描述" className="w-full px-3 py-2 border border-stone-300 rounded-lg text-sm focus:ring-2 focus:ring-stone-400" />
              </div>
              <div className="border-t pt-4">
                <div className="flex items-center justify-between mb-2">
                  <label className="text-sm font-semibold text-stone-600">其他字段</label>
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
              <button onClick={() => setShowFrontmatterDrawer(false)} className="px-4 py-2 border border-stone-300 rounded-lg text-stone-700 hover:bg-stone-50">取消</button>
              <button onClick={applyFrontmatter} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">应用</button>
            </div>
          </div>
        </>
      )}

      {/* 引用搜索弹窗 */}
      {showRefModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setShowRefModal(false)}>
          <div className="bg-white rounded-xl shadow-2xl w-full max-w-lg mx-4 overflow-hidden" onClick={(e) => e.stopPropagation()}>
            <div className="p-4 border-b">
              <h3 className="text-lg font-medium mb-3">搜索文章引用</h3>
              <input
                type="text"
                value={refSearchQuery}
                onChange={(e) => { setRefSearchQuery(e.target.value); searchRefs(); }}
                placeholder="输入关键词搜索文章..."
                className="w-full px-4 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-stone-400 outline-none"
                autoFocus
              />
            </div>
            <div className="max-h-80 overflow-y-auto">
              {refSearchResults.map((item) => (
                <div key={item.path} onClick={() => insertRef(item)} className="px-4 py-3 hover:bg-blue-50 cursor-pointer border-b border-stone-100 flex items-center gap-3">
                  <span className="text-blue-500 text-lg">🔗</span>
                  <div className="min-w-0">
                    <div className="font-medium text-stone-800 truncate">{item.title || '(无标题)'}</div>
                    <div className="text-sm text-stone-400 truncate">{item.path}</div>
                  </div>
                </div>
              ))}
              {refSearchQuery && refSearchResults.length === 0 && (
                <div className="p-6 text-center text-stone-400">未找到匹配的文章</div>
              )}
              {!refSearchQuery && (
                <div className="p-6 text-center text-stone-400">输入关键词开始搜索</div>
              )}
            </div>
            <div className="p-3 border-t bg-stone-50 flex justify-end">
              <button onClick={() => setShowRefModal(false)} className="px-4 py-2 text-stone-600 hover:text-stone-800">取消</button>
            </div>
          </div>
        </div>
      )}

      {/* 反向链接面板 */}
      {showBacklinks && (
        <>
          <div className="fixed inset-0 bg-black/30 z-[55]" onClick={() => setShowBacklinks(false)} />
          <div className="fixed top-0 right-0 w-[380px] max-w-[90vw] h-screen bg-white shadow-xl z-[60] overflow-y-auto flex flex-col">
            <div className="flex items-center justify-between p-4 border-b">
              <h3 className="text-lg font-medium text-stone-900">反向链接</h3>
              <button onClick={() => setShowBacklinks(false)} className="text-stone-400 hover:text-stone-600">
                <X className="w-6 h-6" />
              </button>
            </div>
            {backlinks.length === 0 && <div className="p-4 text-sm text-stone-500">暂无其他文章引用本文</div>}
            {backlinks.map((bl) => (
              <Link key={bl.path} to={`/editor/${bl.path}`} target="_blank" className="block p-4 border-b border-stone-100 hover:bg-stone-50 transition-colors">
                <div className="font-medium text-stone-900">{bl.title || '(无标题)'}</div>
                <div className="text-xs text-stone-400">{bl.path}</div>
                {bl.context && <div className="text-xs text-stone-500 bg-stone-100 p-2 rounded mt-2">{bl.context}</div>}
              </Link>
            ))}
          </div>
        </>
      )}

      {/* 加载状态 */}
      {loading && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-8 text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mb-4" />
            <p className="text-stone-700">加载中...</p>
          </div>
        </div>
      )}

      {/* 冲突对话框 */}
      {conflictRemoteContent !== null && (
        <ConflictModal
          localContent={content}
          remoteContent={conflictRemoteContent}
          onSaveForce={() => saveFile(true)}
          onDiscard={discardAndReload}
          onClose={() => setConflictRemoteContent(null)}
        />
      )}
    </div>
  );
}
