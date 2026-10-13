import type { NavigateFunction } from 'react-router-dom';

import { get, post } from './api';
import { toast } from './toast';

// 仪表板快捷操作与 ⌘K 命令面板共用的真实动作。

export async function createNewPost(navigate: NavigateFunction): Promise<void> {
  const title = prompt('请输入文章标题:');
  if (!title) return;
  try {
    const data = await post<{ success: boolean; path?: string; message?: string }>('/api/post/create', { title });
    if (data.success && data.path) {
      toast('文章创建成功');
      navigate(`/editor/${data.path}`);
    } else {
      toast('创建失败: ' + (data.message || ''));
    }
  } catch {
    toast('创建失败');
  }
}

export async function publishSystem(): Promise<void> {
  try {
    const status = await get<{ success: boolean; has_changes?: boolean; message?: string }>('/api/git/status');
    if (!status.success) {
      toast('Git 状态检查失败: ' + status.message);
      return;
    }
    if (!status.has_changes) {
      toast('没有需要发布的改动');
      return;
    }
    const message = prompt('请输入提交消息 (可选，留空使用默认消息):');
    if (message === null) return;
    const result = await post<{ success: boolean; message?: string }>('/api/publish/system', { message: message || null });
    toast(result.success ? '系统发布成功！GitHub Actions 将自动构建站点' : '发布失败: ' + (result.message || ''));
  } catch {
    toast('系统发布失败');
  }
}

interface EmailPreview {
  success: boolean;
  data?: { is_already_sent: boolean; post: { title: string; link: string }; subject: string };
  message?: string;
}

async function pushWithConfirm(preview: EmailPreview, push: (force: boolean) => Promise<{ success: boolean; message?: string }>): Promise<void> {
  if (!preview.success || !preview.data) {
    toast('获取预览失败: ' + (preview.message || ''));
    return;
  }
  const p = preview.data;
  const alreadySent = p.is_already_sent ? '（该文章已推送过）' : '';
  const confirmed = confirm(
    `确定要推送这篇文章给订阅者吗？${alreadySent}\n\n标题：${p.post.title}\n链接：${p.post.link}\n\n邮件主题：${p.subject}`,
  );
  if (!confirmed) return;
  const force = p.is_already_sent ? confirm('该文章已推送过，是否强制再次推送？') : false;
  const result = await push(force);
  toast(result.success ? '邮件推送成功！' : '推送失败: ' + (result.message || ''));
}

export async function pushLatestEmail(): Promise<void> {
  try {
    const preview = await get<EmailPreview>('/api/email/preview-latest');
    await pushWithConfirm(preview, (force) =>
      post<{ success: boolean; message?: string }>('/api/email/push-latest', { force, debug_mode: false }),
    );
  } catch {
    toast('邮件推送失败');
  }
}

export async function pushSpecificEmail(): Promise<void> {
  const url = prompt('请输入要推送的文章 URL 或路径：\n\n例如：\n- https://svtter.cn/post/xxx/\n- /post/xxx/');
  if (!url) return;
  try {
    const preview = await get<EmailPreview>('/api/email/preview-article?url=' + encodeURIComponent(url));
    await pushWithConfirm(preview, (force) =>
      post<{ success: boolean; message?: string }>('/api/email/push-article', { url, force, debug_mode: false }),
    );
  } catch {
    toast('邮件推送失败');
  }
}
