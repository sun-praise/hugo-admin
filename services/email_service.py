# coding: utf-8
"""
邮件推送服务
负责获取最新文章并推送给订阅者
移植自 rss2email-automation/send_latest_post.py
"""

import re
import json
import smtplib
from pathlib import Path
from datetime import datetime
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart

import yaml
import requests
import feedparser


class EmailService:
    """邮件推送服务"""

    def __init__(self, config_file=None, debug_mode=False):
        """
        初始化邮件服务

        Args:
            config_file: 配置文件路径，默认为 ~/.config/secret.yml
            debug_mode: 调试模式，True 时发送到 MailHog 而非真实用户
        """
        if config_file is None:
            config_file = Path.home() / ".config" / "secret.yml"

        self.debug_mode = debug_mode
        self.config_file = Path(config_file)

        # 加载配置
        self._load_config()

        # RSS 配置
        self.rss_url = "https://svtter.cn/index.xml"
        self.sent_file = Path.home() / ".config" / "latest_post_sent.json"

        # Debug 模式配置
        self.debug_smtp_host = "localhost"
        self.debug_smtp_port = 1025
        self.debug_test_email = "test@example.com"

    def _load_config(self):
        """加载配置文件"""
        if not self.config_file.exists():
            raise FileNotFoundError(f"配置文件不存在: {self.config_file}")

        with open(self.config_file, "r", encoding="utf-8") as f:
            config = yaml.safe_load(f)

        self.listmonk = config.get("listmonk", {})
        self.api_url = self.listmonk.get("api_url", "")
        self.api_user = self.listmonk.get("api_user", "")
        self.api_key = self.listmonk.get("api_key", "")
        self.list_id = self.listmonk.get("blog_list_id", 1)

        # API 认证头
        self.headers = {
            "Authorization": f"token {self.api_user}:{self.api_key}",
            "Content-Type": "application/json",
        }

    def get_latest_post(self):
        """
        获取最新文章

        Returns:
            dict: 包含文章信息的字典，失败返回 None
        """
        try:
            feed = feedparser.parse(self.rss_url)

            if feed.bozo:
                # RSS 解析有警告但可能仍有数据
                pass

            if not feed.entries:
                return None

            latest = feed.entries[0]

            # 转换为字典格式
            return {
                "title": latest.title,
                "link": latest.link,
                "published": getattr(latest, "published", ""),
                "description": getattr(latest, "description", ""),
                "summary": getattr(latest, "summary", ""),
                "tags": [tag.term for tag in getattr(latest, "tags", [])],
            }

        except Exception as e:
            raise Exception(f"获取 RSS 失败: {e}")

    def get_post_by_url(self, url):
        """
        根据 URL 获取指定文章

        Args:
            url: 文章 URL 或路径

        Returns:
            dict: 包含文章信息的字典，失败返回 None
        """
        try:
            feed = feedparser.parse(self.rss_url)

            if not feed.entries:
                return None

            # 标准化 URL 进行匹配
            target_url = url.rstrip("/")

            for entry in feed.entries:
                entry_url = entry.link.rstrip("/")
                # 支持完整 URL 或路径匹配
                if entry_url == target_url or entry_url.endswith(target_url):
                    return {
                        "title": entry.title,
                        "link": entry.link,
                        "published": getattr(entry, "published", ""),
                        "description": getattr(entry, "description", ""),
                        "summary": getattr(entry, "summary", ""),
                        "tags": [tag.term for tag in getattr(entry, "tags", [])],
                    }

            return None

        except Exception as e:
            raise Exception(f"获取文章失败: {e}")

    def load_sent_record(self):
        """加载已发送记录"""
        if self.sent_file.exists():
            try:
                with open(self.sent_file, "r", encoding="utf-8") as f:
                    return json.load(f)
            except (json.JSONDecodeError, IOError):
                return {}
        return {}

    def save_sent_record(self, record):
        """保存已发送记录"""
        with open(self.sent_file, "w", encoding="utf-8") as f:
            json.dump(record, f, ensure_ascii=False, indent=2)

    def is_already_sent(self, post):
        """检查文章是否已发送过"""
        record = self.load_sent_record()
        post_id = post["link"]
        return post_id in record.get("sent_posts", [])

    def create_email_content(self, post):
        """
        创建邮件内容

        Args:
            post: 文章信息字典

        Returns:
            tuple: (subject, body_html)
        """
        # 邮件主题
        subject = f"📖 新文章发布：{post['title']}"

        # 提取文章摘要
        description = post.get("description", "")
        summary = post.get("summary", description)

        # 清理 HTML 标签并截取合适长度
        clean_summary = re.sub(r"<[^>]+>", "", summary)
        if len(clean_summary) > 400:
            clean_summary = clean_summary[:400] + "..."

        # 获取文章标签
        tags = post.get("tags", [])
        category_html = ""
        if tags:
            category_tags = " ".join(
                [
                    f'<span style="background: #e3f2fd; color: #1976d2; padding: 2px 8px; border-radius: 12px; font-size: 12px; margin-right: 5px;">#{tag}</span>'
                    for tag in tags[:3]
                ]
            )
            category_html = f'<div style="margin-bottom: 15px;">{category_tags}</div>'

        # HTML 邮件模板
        body = f"""
        <!DOCTYPE html>
        <html lang="zh-CN">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <title>{subject}</title>
        </head>
        <body style="margin: 0; padding: 0; background-color: #f5f5f5; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;">
            <div style="max-width: 600px; margin: 20px auto; background: white; border-radius: 12px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); overflow: hidden;">
                
                <!-- 邮件头部 -->
                <div style="background: linear-gradient(135deg, #007cba 0%, #0056b3 100%); padding: 30px; text-align: center;">
                    <h1 style="color: white; margin: 0; font-size: 24px; font-weight: 600;">📖 Svtter's Blog</h1>
                    <p style="color: rgba(255,255,255,0.9); margin: 8px 0 0 0; font-size: 14px;">有新文章发布啦！</p>
                </div>
                
                <!-- 文章内容 -->
                <div style="padding: 30px;">
                    <div style="border-left: 4px solid #007cba; padding-left: 20px; margin-bottom: 25px;">
                        <h2 style="color: #333; margin: 0 0 10px 0; font-size: 22px; line-height: 1.4;">{post["title"]}</h2>
                        <p style="color: #666; font-size: 14px; margin: 0;">
                            📅 {post.get("published", "")} &nbsp;&nbsp; 
                            👁️ <a href="https://svtter.cn" style="color: #007cba; text-decoration: none;">svtter.cn</a>
                        </p>
                    </div>
                    
                    {category_html}
                    
                    <div style="background: #f8f9fa; padding: 25px; border-radius: 8px; border: 1px solid #e9ecef; margin: 20px 0;">
                        <div style="color: #333; line-height: 1.7; font-size: 15px;">
                            {clean_summary}
                        </div>
                    </div>
                    
                    <div style="text-align: center; margin: 35px 0;">
                        <a href="{post["link"]}" 
                           style="display: inline-block; 
                                  background: linear-gradient(135deg, #007cba 0%, #0056b3 100%); 
                                  color: white; 
                                  padding: 15px 35px; 
                                  text-decoration: none; 
                                  border-radius: 25px; 
                                  font-weight: 600;
                                  font-size: 16px;
                                  box-shadow: 0 3px 10px rgba(0, 124, 186, 0.3);
                                  transition: all 0.3s ease;">
                            📖 立即阅读全文
                        </a>
                    </div>
                    
                    <div style="background: #e8f4fd; padding: 20px; border-radius: 8px; margin-top: 30px;">
                        <p style="color: #1565c0; font-size: 14px; margin: 0; text-align: center;">
                            💡 喜欢这篇文章？别忘了分享给朋友们！
                        </p>
                    </div>
                </div>
                
                <!-- 邮件底部 -->
                <div style="background: #f8f9fa; padding: 25px; border-top: 1px solid #e9ecef; text-align: center;">
                    <p style="color: #666; font-size: 13px; margin: 0 0 10px 0;">
                        您收到此邮件是因为您订阅了 <a href="https://svtter.cn" style="color: #007cba; text-decoration: none;">Svtter's Blog</a>
                    </p>
                    <p style="color: #999; font-size: 12px; margin: 0;">
                        <a href="{self.api_url}/subscription/form" style="color: #999; text-decoration: none;">管理订阅</a> | 
                        <a href="https://svtter.cn" style="color: #999; text-decoration: none;">访问博客</a>
                    </p>
                </div>
            </div>
            
            <!-- 邮件底部空白 -->
            <div style="height: 20px;"></div>
        </body>
        </html>
        """

        return subject, body

    @staticmethod
    def normalize_listmonk_template_vars(content):
        """
        兼容旧版模板变量写法。

        listmonk 在模板中通过函数暴露部分变量（例如 UnsubscribeURL），
        旧写法 `{{ .UnsubscribeURL }}` 会导致渲染失败。
        """
        if not content:
            return content

        replacements = {
            r"{{\s*\.UnsubscribeURL\s*}}": "{{ UnsubscribeURL }}",
            r"{{\s*\.MessageURL\s*}}": "{{ MessageURL }}",
            r"{{\s*\.TrackView\s*}}": "{{ TrackView }}",
        }

        normalized = content
        for pattern, replacement in replacements.items():
            normalized = re.sub(pattern, replacement, normalized)

        # 兼容 `{{ .TrackLink "..." }}` -> `{{ TrackLink "..." }}`
        normalized = re.sub(r"{{\s*\.TrackLink(\s+[^}]*)}}", r"{{ TrackLink\1}}", normalized)
        return normalized

    def preview_email(self, post):
        """
        预览邮件内容（不发送）

        Args:
            post: 文章信息字典

        Returns:
            dict: 预览信息
        """
        subject, body = self.create_email_content(post)

        return {
            "subject": subject,
            "body_html": body,
            "post": post,
            "is_already_sent": self.is_already_sent(post),
            "debug_mode": self.debug_mode,
            "target_list_id": self.list_id,
        }

    def send_debug_email(self, post):
        """
        调试模式：通过 SMTP 发送邮件到 MailHog

        Args:
            post: 文章信息字典

        Returns:
            dict: 发送结果
        """
        try:
            subject, body = self.create_email_content(post)
            body = self.normalize_listmonk_template_vars(body)

            # 创建邮件对象
            msg = MIMEMultipart("alternative")
            msg["Subject"] = f"[DEBUG] {subject}"
            msg["From"] = "debug@svtter.cn"
            msg["To"] = self.debug_test_email
            msg["Date"] = datetime.now().strftime("%a, %d %b %Y %H:%M:%S +0800")

            # 添加 HTML 内容
            html_part = MIMEText(body, "html", "utf-8")
            msg.attach(html_part)

            # 连接到 MailHog SMTP 服务器
            with smtplib.SMTP(self.debug_smtp_host, self.debug_smtp_port) as server:
                server.send_message(msg)

            return {
                "success": True,
                "message": "调试邮件发送成功",
                "details": {
                    "subject": subject,
                    "title": post["title"],
                    "to": self.debug_test_email,
                    "mailhog_url": "http://localhost:8025",
                },
            }

        except Exception as e:
            return {
                "success": False,
                "message": f"调试邮件发送失败: {e}",
                "hint": "请确保 MailHog 容器正在运行：docker-compose up -d",
            }

    def send_campaign(self, post, force=False):
        """
        发送邮件活动（真实发送给订阅者）

        Args:
            post: 文章信息字典
            force: 是否强制发送（即使已发送过）

        Returns:
            dict: 发送结果
        """
        try:
            # 检查是否已发送过
            if not force and self.is_already_sent(post):
                return {
                    "success": False,
                    "message": f"文章已发送过: {post['title']}",
                    "hint": "如需强制发送，请设置 force=true",
                }

            # 创建邮件内容
            subject, body = self.create_email_content(post)

            # 活动数据
            campaign_data = {
                "name": f"📖 {post['title']} - {datetime.now().strftime('%Y%m%d_%H%M')}",
                "subject": subject,
                "lists": [self.list_id],
                "type": "regular",
                "content_type": "html",
                "body": body,
                "tags": ["blog", "latest", "auto-send"],
            }

            # 创建活动
            url = f"{self.api_url}/api/campaigns"
            response = requests.post(url, headers=self.headers, json=campaign_data)
            response.raise_for_status()

            campaign = response.json()["data"]
            campaign_id = campaign["id"]

            # 发送活动
            send_url = f"{self.api_url}/api/campaigns/{campaign_id}/status"
            send_data = {"status": "running"}

            response = requests.put(send_url, headers=self.headers, json=send_data)
            response.raise_for_status()

            # 记录已发送
            record = self.load_sent_record()
            if "sent_posts" not in record:
                record["sent_posts"] = []

            record["sent_posts"].append(post["link"])
            record["last_sent"] = {
                "title": post["title"],
                "link": post["link"],
                "sent_time": datetime.now().isoformat(),
                "campaign_id": campaign_id,
            }

            self.save_sent_record(record)

            return {
                "success": True,
                "message": "邮件发送成功",
                "details": {
                    "campaign_name": campaign_data["name"],
                    "subject": subject,
                    "campaign_id": campaign_id,
                    "list_id": self.list_id,
                },
            }

        except requests.exceptions.RequestException as e:
            error_detail = ""
            if hasattr(e, "response") and e.response is not None:
                error_detail = e.response.text
            return {
                "success": False,
                "message": f"发送失败: {e}",
                "error_detail": error_detail,
            }
        except Exception as e:
            return {"success": False, "message": f"发送失败: {e}"}

    def push_latest(self, force=False):
        """
        推送最新文章

        Args:
            force: 是否强制发送

        Returns:
            dict: 发送结果
        """
        post = self.get_latest_post()
        if not post:
            return {"success": False, "message": "无法获取最新文章"}

        if self.debug_mode:
            result = self.send_debug_email(post)
        else:
            result = self.send_campaign(post, force)

        result["post"] = post
        return result

    def push_article(self, url, force=False):
        """
        推送指定文章

        Args:
            url: 文章 URL 或路径
            force: 是否强制发送

        Returns:
            dict: 发送结果
        """
        post = self.get_post_by_url(url)
        if not post:
            return {"success": False, "message": f"未找到文章: {url}"}

        if self.debug_mode:
            result = self.send_debug_email(post)
        else:
            result = self.send_campaign(post, force)

        result["post"] = post
        return result

    def preview_latest(self):
        """
        预览最新文章邮件

        Returns:
            dict: 预览信息
        """
        post = self.get_latest_post()
        if not post:
            return {"success": False, "message": "无法获取最新文章"}

        preview = self.preview_email(post)
        return {"success": True, "message": "预览生成成功", "data": preview}

    def preview_article(self, url):
        """
        预览指定文章邮件

        Args:
            url: 文章 URL 或路径

        Returns:
            dict: 预览信息
        """
        post = self.get_post_by_url(url)
        if not post:
            return {"success": False, "message": f"未找到文章: {url}"}

        preview = self.preview_email(post)
        return {"success": True, "message": "预览生成成功", "data": preview}
