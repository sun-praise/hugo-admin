export interface Post {
  path: string;
  title: string;
  date: string;
  excerpt: string;
  description?: string;
  tags: string[];
  categories: string[];
  cover_url?: string;
  mod_time?: string;
  status?: {
    is_draft: boolean;
  };
}

export interface Pagination {
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface PostsResponse {
  posts: Post[];
  total: number;
  pagination: Pagination;
}

export interface Tag {
  name: string;
  count: number;
}

export interface Category {
  name: string;
  count: number;
}

export interface ServerStatus {
  running: boolean;
  pid: number | null;
  uptime?: string;
  cpu_percent?: number;
  memory_mb?: number;
}

export interface Settings {
  hugo: {
    base_dir: string;
    server_url: string;
  };
  ai: {
    base_url: string;
    model: string;
    api_key?: string;
    api_key_source?: string;
    api_key_hint?: string;
  };
}

export interface Frontmatter {
  title?: string;
  date?: string;
  draft?: boolean;
  tags?: string[];
  categories?: string[];
  cover?: string;
  description?: string;
  [key: string]: unknown;
}

export interface FileData {
  content: string;
  frontmatter?: Record<string, unknown>;
}

export interface ImageItem {
  name: string;
  url: string;
}

export interface Backlink {
  path: string;
  title: string;
  context?: string;
}

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface ChatSession {
  session_id: string;
  title: string;
}

export interface LogEntry {
  timestamp: string;
  level: 'INFO' | 'SUCCESS' | 'WARNING' | 'ERROR';
  message: string;
}
