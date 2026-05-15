import { useState, useRef, useEffect, useCallback } from 'react';
import { useLocation } from 'react-router-dom';
import { MessageSquare, X, ChevronDown, Plus, Trash2 } from 'lucide-react';
import { renderMarkdown, escapeHtml } from '../utils/markdown';
import { get, post, del as deleteReq } from '../utils/api';
import type { ChatMessage, ChatSession } from '../types';

function formatToolCard(tool: string, args: unknown): string {
  const argsStr = typeof args === 'string' ? args : JSON.stringify(args, null, 2);
  return `<div class="ai-tool-card">
    <div class="ai-tool-card-header">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
      </svg>
      <span>调用工具: ${escapeHtml(tool)}</span>
    </div>
    <div class="ai-tool-card-body"><pre>${escapeHtml(argsStr)}</pre></div>
  </div>`;
}

function formatResultCard(result: unknown): string {
  const resultStr = typeof result === 'string' ? result : JSON.stringify(result, null, 2);
  return `<div class="ai-result-card">
    <div class="ai-result-card-header">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
      </svg>
      <span>执行结果</span>
    </div>
    <div class="ai-result-card-body"><pre>${escapeHtml(resultStr)}</pre></div>
  </div>`;
}

function formatErrorCard(error: string): string {
  return `<div class="ai-error-card">
    <div class="ai-error-card-header">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
      </svg>
      <span>错误: ${escapeHtml(error)}</span>
    </div>
  </div>`;
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

  async function createSession() {
    try {
      const res = await post<{ session_id: string }>('/api/ai/sessions', {});
      setCurrentSessionId(res.session_id);
      setMessages([
        { role: 'assistant', content: '您好！我是您的 Hugo 博客 AI 助手（只读模式）。我可以帮您搜索和阅读文章，或回答相关问题。' },
      ]);
      await loadSessions();
    } catch (e) {
      console.error('Failed to create session:', e);
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

    if (!currentSessionId) {
      await createSession();
    }

    const userMessage = userInput.trim();
    setUserInput('');
    setMessages((prev) => [...prev, { role: 'user', content: userMessage }]);
    setIsLoading(true);

    try {
      const response = await fetch('/api/ai/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: userMessage,
          history: messages,
          session_id: currentSessionId,
          current_file: currentFile || undefined,
        }),
      });

      if (!response.ok) throw new Error('Network response was not ok');

      const reader = response.body!.getReader();
      const decoder = new TextDecoder();
      let accumulatedContent = '';

      setMessages((prev) => [...prev, { role: 'assistant', content: '' }]);

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');

        for (const line of lines) {
          const trimmedLine = line.trim();
          if (trimmedLine.startsWith('data:')) {
            const data = trimmedLine.slice(5).trim();
            if (data === '[DONE]') break;

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
      }
    } catch (error) {
      console.error('Chat error:', error);
      setMessages((prev) => [
        ...prev,
        {
          role: 'assistant',
          content: '抱歉，处理您的请求时发生了错误：' + (error as Error).message,
        },
      ]);
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col items-end">
      {isOpen && (
        <div className="bg-white w-96 h-[500px] rounded-2xl shadow-2xl border border-gray-200 flex flex-col mb-4 overflow-hidden fade-in">
          <div className="bg-gray-900 text-white px-4 py-3 flex justify-between items-center">
            <div className="flex items-center space-x-2">
              <div className="w-2 h-2 bg-green-500 rounded-full" />
              <span className="font-medium text-sm">AI 助手</span>
              <div className="relative">
                <button
                  onClick={() => setShowMenu(!showMenu)}
                  className="ml-2 text-gray-400 hover:text-white text-xs flex items-center"
                >
                  <ChevronDown className="w-4 h-4" />
                </button>
                {showMenu && (
                  <div className="absolute left-0 mt-2 w-48 bg-white rounded-lg shadow-lg z-50 py-1 text-gray-800 text-sm max-h-64 overflow-y-auto">
                    <button
                      onClick={() => {
                        createSession();
                        setShowMenu(false);
                      }}
                      className="w-full px-3 py-2 text-left hover:bg-gray-100 flex items-center text-blue-600"
                    >
                      <Plus className="w-4 h-4 mr-2" />
                      新对话
                    </button>
                    <div className="border-t border-gray-200 my-1" />
                    {sessions.map((s) => (
                      <div
                        key={s.session_id}
                        className="flex items-center hover:bg-gray-100 group"
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
                          className="px-2 text-gray-400 hover:text-red-500 opacity-0 group-hover:opacity-100"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    ))}
                    {sessions.length === 0 && (
                      <div className="px-3 py-2 text-gray-400 text-center">暂无历史</div>
                    )}
                  </div>
                )}
              </div>
            </div>
            <button onClick={() => setIsOpen(false)} className="text-gray-400 hover:text-white">
              <X className="w-5 h-5" />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto p-4 space-y-4 bg-gray-50">
            {messages.map((msg, index) => (
              <div
                key={index}
                className={msg.role === 'user' ? 'flex justify-end' : 'flex justify-start'}
              >
                <div
                  className={`max-w-[85%] px-4 py-2 rounded-2xl shadow-sm text-sm ${
                    msg.role === 'user'
                      ? 'bg-blue-600 text-white rounded-br-none'
                      : 'bg-white text-gray-800 border border-gray-200 rounded-bl-none'
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
                <div className="bg-white border border-gray-200 px-4 py-2 rounded-2xl rounded-bl-none shadow-sm flex items-center space-x-2">
                  <div className="flex space-x-1">
                    <div className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" />
                    <div className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '0.2s' }} />
                    <div className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '0.4s' }} />
                  </div>
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          <form onSubmit={sendMessage} className="p-4 bg-white border-t border-gray-200">
            <div className="flex space-x-2">
              <input
                type="text"
                value={userInput}
                onChange={(e) => setUserInput(e.target.value)}
                placeholder="输入指令，例如：搜索文章..."
                className="flex-1 bg-gray-100 border-none rounded-full px-4 py-2 text-sm focus:ring-2 focus:ring-blue-500 outline-none"
                disabled={isLoading}
              />
              <button
                type="submit"
                disabled={isLoading || !userInput.trim()}
                className="bg-blue-600 text-white p-2 rounded-full hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
                </svg>
              </button>
            </div>
          </form>
        </div>
      )}

      <button
        onClick={() => setIsOpen(!isOpen)}
        className="bg-gray-900 text-white p-4 rounded-full shadow-2xl hover:scale-110 transition-transform flex items-center justify-center relative"
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
