import { useState, useRef, useEffect, useCallback } from 'react';
import { useLocation } from 'react-router-dom';
import { MessageSquare, X, ChevronDown, Plus, Trash2, StopCircle } from 'lucide-react';
import { renderMarkdown, escapeHtml } from '../utils/markdown';
import { get, post, del as deleteReq } from '../utils/api';
import type { ChatMessage, ChatSession } from '../types';

function formatToolCard(tool: string, args: unknown): string {
  const argsStr = typeof args === 'string' ? args : JSON.stringify(args, null, 2);
  return `<details class="ai-tool-card">
    <summary class="ai-tool-card-header">
      <svg class="ai-tool-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
      </svg>
      <span>调用工具: ${escapeHtml(tool)}</span>
      <svg class="ai-tool-chevron" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
      </svg>
    </summary>
    <div class="ai-tool-card-body"><pre>${escapeHtml(argsStr)}</pre></div>
  </details>`;
}

function extractText(result: unknown): string {
  if (typeof result === 'string') return result;
  if (Array.isArray(result)) {
    return result
      .map((item: unknown) => {
        if (typeof item === 'string') return item;
        if (item && typeof item === 'object' && 'text' in item) return (item as { text: string }).text;
        return JSON.stringify(item);
      })
      .join('\n');
  }
  if (result && typeof result === 'object' && 'text' in result) return (result as { text: string }).text;
  return JSON.stringify(result, null, 2);
}

function formatResultCard(result: unknown): string {
  const text = extractText(result);
  const preview = text.length > 60 ? text.slice(0, 60) + '…' : text;
  return `<details class="ai-result-card">
    <summary class="ai-result-card-header">
      <svg class="ai-tool-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
      </svg>
      <span class="ai-result-preview">${escapeHtml(preview)}</span>
      <svg class="ai-tool-chevron" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
      </svg>
    </summary>
    <div class="ai-tool-card-body"><pre>${escapeHtml(text)}</pre></div>
  </details>`;
}

function formatErrorCard(error: string): string {
  return `<details class="ai-error-card" open>
    <summary class="ai-error-card-header">
      <svg class="ai-tool-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
      </svg>
      <span>错误: ${escapeHtml(error)}</span>
      <svg class="ai-tool-chevron" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
      </svg>
    </summary>
  </details>`;
}

export default function AIChat() {
  const location = useLocation();
  const [isOpen, setIsOpen] = useState(false);
  const [userInput, setUserInput] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([
    { role: 'assistant', content: '您好！我是您的 Hugo 博客 AI 助手（只读模式）。我可以帮您搜索和阅读文章，或回答相关问题。' },
  ]);
  const [isLoading, setIsLoading] = useState(false);
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [showMenu, setShowMenu] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const currentFile = location.pathname.startsWith('/editor/')
    ? location.pathname.replace('/editor/', '')
    : '';

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, isLoading, scrollToBottom]);

  async function loadSessions() {
    try {
      const res = await get<{ sessions: ChatSession[] }>('/api/ai/sessions');
      setSessions(res.sessions || []);
    } catch (e) {
      console.error('Failed to load sessions:', e);
    }
  }

  useEffect(() => {
    loadSessions();
  }, []);

  async function createSession(): Promise<string | null> {
    try {
      const res = await post<{ session_id: string }>('/api/ai/sessions', {});
      setCurrentSessionId(res.session_id);
      setMessages([
        { role: 'assistant', content: '您好！我是您的 Hugo 博客 AI 助手（只读模式）。我可以帮您搜索和阅读文章，或回答相关问题。' },
      ]);
      await loadSessions();
      return res.session_id;
    } catch (e) {
      console.error('Failed to create session:', e);
      return null;
    }
  }

  async function selectSession(sessionId: string) {
    try {
      const res = await get<{ messages: { role: string; content: string }[] }>(`/api/ai/sessions/${sessionId}`);
      setCurrentSessionId(sessionId);
      setMessages(
        res.messages && res.messages.length > 0
          ? res.messages.map((m) => ({ role: m.role as 'user' | 'assistant', content: m.content }))
          : [{ role: 'assistant', content: '您好！我是您的 Hugo 博客 AI 助手（只读模式）。' }],
      );
    } catch (e) {
      console.error('Failed to load session:', e);
    }
  }

  async function deleteSession(sessionId: string) {
    if (!confirm('确定要删除这个对话吗？')) return;
    try {
      await deleteReq(`/api/ai/sessions/${sessionId}`);
      if (currentSessionId === sessionId) {
        setCurrentSessionId(null);
        setMessages([{ role: 'assistant', content: '您好！我是您的 Hugo 博客 AI 助手（只读模式）。' }]);
      }
      await loadSessions();
    } catch (e) {
      console.error('Failed to delete session:', e);
    }
  }

  async function sendMessage(e: React.FormEvent) {
    e.preventDefault();
    if (!userInput.trim() || isLoading) return;

    let sessionId = currentSessionId;
    if (!sessionId) {
      sessionId = await createSession();
      if (!sessionId) {
        setMessages((prev) => [...prev, { role: 'assistant', content: '抱歉，会话创建失败，请稍后重试。' }]);
        return;
      }
    }

    const userMessage = userInput.trim();
    setUserInput('');
    setMessages((prev) => [...prev, { role: 'user', content: userMessage }]);
    setIsLoading(true);

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    try {
      const response = await fetch('/api/ai/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: userMessage,
          history: messages,
          session_id: sessionId,
          current_file: currentFile || undefined,
        }),
        signal: abortController.signal,
      });

      if (!response.ok) throw new Error('Network response was not ok');

      const reader = response.body!.getReader();
      const decoder = new TextDecoder();
      let accumulatedContent = '';
      let sseBuffer = '';

      setMessages((prev) => [...prev, { role: 'assistant', content: '' }]);

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        sseBuffer += decoder.decode(value, { stream: true });
        const parts = sseBuffer.split('\n\n');
        // Last element may be incomplete; keep it in buffer
        sseBuffer = parts.pop()!;

        for (const part of parts) {
          // Each "part" is one SSE event block (may contain multiple data: lines)
          const dataLines = part.split('\n')
            .map((l: string) => l.trim())
            .filter((l: string) => l.startsWith('data:'))
            .map((l: string) => l.slice(5).trim());

          if (dataLines.length === 0) continue;
          if (dataLines.includes('[DONE]')) break;

          const data = dataLines.join('');

          try {
            const parsed = JSON.parse(data);
            if (parsed.type === 'tool_call') {
              accumulatedContent += formatToolCard(parsed.tool, parsed.args);
            } else if (parsed.type === 'tool_result') {
              accumulatedContent += formatResultCard(parsed.result);
            } else if (parsed.type === 'error') {
              accumulatedContent += formatErrorCard(parsed.error);
            }
          } catch {
            accumulatedContent += data;
          }

          setMessages((prev) => {
            const newMessages = [...prev];
            newMessages[newMessages.length - 1] = {
              role: 'assistant',
              content: accumulatedContent,
            };
            return newMessages;
          });
        }
      }
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        setMessages((prev) => {
          const newMessages = [...prev];
          const last = newMessages[newMessages.length - 1];
          if (last.role === 'assistant' && last.content === '') {
            newMessages[newMessages.length - 1] = {
              role: 'assistant',
              content: '（已停止生成）',
            };
          }
          return newMessages;
        });
      } else {
        console.error('Chat error:', error);
        setMessages((prev) => [
          ...prev,
          {
            role: 'assistant',
            content: '抱歉，处理您的请求时发生了错误：' + (error as Error).message,
          },
        ]);
      }
    } finally {
      setIsLoading(false);
      abortControllerRef.current = null;
    }
  }

  function stopGeneration() {
    abortControllerRef.current?.abort();
  }

  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col items-end">
      {isOpen && (
        <div className="bg-white w-96 h-[500px] rounded-2xl shadow-2xl border border-stone-200 flex flex-col mb-4 overflow-hidden fade-in">
          <div className="bg-stone-900 text-white px-4 py-3 flex justify-between items-center">
            <div className="flex items-center space-x-2">
              <div className="w-2 h-2 bg-green-500 rounded-full" />
              <span className="font-medium text-sm">AI 助手</span>
              <div className="relative">
                <button
                  onClick={() => setShowMenu(!showMenu)}
                  className="ml-2 text-stone-400 hover:text-white text-xs flex items-center"
                >
                  <ChevronDown className="w-4 h-4" />
                </button>
                {showMenu && (
                  <div className="absolute left-0 mt-2 w-48 bg-white rounded-lg shadow-lg z-50 py-1 text-stone-800 text-sm max-h-64 overflow-y-auto">
                    <button
                      onClick={() => {
                        createSession();
                        setShowMenu(false);
                      }}
                      className="w-full px-3 py-2 text-left hover:bg-stone-100 flex items-center text-blue-600"
                    >
                      <Plus className="w-4 h-4 mr-2" />
                      新对话
                    </button>
                    <div className="border-t border-stone-200 my-1" />
                    {sessions.map((s) => (
                      <div
                        key={s.session_id}
                        className="flex items-center hover:bg-stone-100 group"
                      >
                        <button
                          onClick={() => {
                            selectSession(s.session_id);
                            setShowMenu(false);
                          }}
                          className={`flex-1 px-3 py-2 text-left truncate ${
                            s.session_id === currentSessionId
                              ? 'bg-blue-50 text-blue-600'
                              : ''
                          }`}
                        >
                          {s.title || '新对话'}
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            deleteSession(s.session_id);
                          }}
                          className="px-2 text-stone-400 hover:text-red-500 opacity-0 group-hover:opacity-100"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    ))}
                    {sessions.length === 0 && (
                      <div className="px-3 py-2 text-stone-400 text-center">暂无历史</div>
                    )}
                  </div>
                )}
              </div>
            </div>
            <button onClick={() => setIsOpen(false)} className="text-stone-400 hover:text-white">
              <X className="w-5 h-5" />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto p-4 space-y-4 bg-stone-50">
            {messages.map((msg, index) => (
              <div
                key={index}
                className={msg.role === 'user' ? 'flex justify-end' : 'flex justify-start'}
              >
                <div
                  className={`max-w-[85%] px-4 py-2 rounded-2xl text-sm ${
                    msg.role === 'user'
                      ? 'bg-blue-600 text-white rounded-br-none'
                      : 'bg-white text-stone-800 border border-stone-200 rounded-bl-none'
                  }`}
                >
                  <div
                    className={`ai-prose max-w-none break-words ${
                      msg.role === 'user' ? 'text-white' : ''
                    }`}
                    dangerouslySetInnerHTML={{ __html: renderMarkdown(msg.content) }}
                  />
                </div>
              </div>
            ))}
            {isLoading && (
              <div className="flex justify-start">
                <div className="bg-white border border-stone-200 px-4 py-2 rounded-2xl rounded-bl-none flex items-center space-x-2">
                  <div className="flex space-x-1">
                    <div className="w-1.5 h-1.5 bg-stone-400 rounded-full animate-bounce" />
                    <div className="w-1.5 h-1.5 bg-stone-400 rounded-full animate-bounce" style={{ animationDelay: '0.2s' }} />
                    <div className="w-1.5 h-1.5 bg-stone-400 rounded-full animate-bounce" style={{ animationDelay: '0.4s' }} />
                  </div>
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          <form onSubmit={sendMessage} className="p-4 bg-white border-t border-stone-200">
            <div className="flex space-x-2">
              <input
                type="text"
                value={userInput}
                onChange={(e) => setUserInput(e.target.value)}
                placeholder="输入指令，例如：搜索文章..."
                className="flex-1 bg-stone-100 border-none rounded-full px-4 py-2 text-sm focus:ring-2 focus:ring-stone-400 outline-none"
                disabled={isLoading}
              />
              {isLoading ? (
                <button
                  type="button"
                  onClick={stopGeneration}
                  className="bg-red-500 text-white p-2 rounded-full hover:bg-red-600 transition-colors"
                  title="停止生成"
                >
                  <StopCircle className="w-5 h-5" />
                </button>
              ) : (
                <button
                  type="submit"
                  disabled={!userInput.trim()}
                  className="bg-blue-600 text-white p-2 rounded-full hover:bg-blue-700 disabled:opacity-50 transition-colors"
                >
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
                  </svg>
                </button>
              )}
            </div>
          </form>
        </div>
      )}

      <button
        onClick={() => setIsOpen(!isOpen)}
        className="bg-stone-900 text-white p-4 rounded-full shadow-2xl hover:scale-110 transition-transform flex items-center justify-center relative"
      >
        {isOpen ? (
          <ChevronDown className="w-6 h-6" />
        ) : (
          <MessageSquare className="w-6 h-6" />
        )}
        {!isOpen && (
          <div className="absolute -top-1 -right-1 w-3 h-3 bg-red-500 rounded-full border-2 border-white" />
        )}
      </button>
    </div>
  );
}
