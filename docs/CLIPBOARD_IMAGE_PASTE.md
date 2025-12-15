# 剪贴板图片粘贴功能

## 功能说明

在 Markdown 编辑器中,您现在可以直接从剪贴板粘贴图片,系统会自动上传并插入图片链接。

## 使用方法

1. **复制图片到剪贴板**:
   - 截图工具 (Windows: Win+Shift+S, macOS: Cmd+Shift+4)
   - 从网页或其他应用复制图片
   - 从文件管理器复制图片文件

2. **粘贴到编辑器**:
   - 在编辑器中点击要插入图片的位置
   - 使用快捷键 `Ctrl+V` (或 `Cmd+V` on macOS) 粘贴
   - 系统会自动检测剪贴板中的图片

3. **自动处理**:
   - 系统显示"正在上传图片..."提示
   - 图片自动上传到文章的 `pics/` 目录
   - 文件名自动生成: `clipboard-{timestamp}.png`
   - 自动在光标位置插入 Markdown 图片语法: `![](pics/图片名.png)`
   - 图片列表自动刷新

## 技术实现

### 前端实现

在 `templates/editor.html` 中:

1. **粘贴事件监听器**:
   ```javascript
   const editor = document.getElementById('markdown-editor');
   editor.addEventListener('paste', (e) => {
       this.handlePaste(e);
   });
   ```

2. **处理剪贴板内容**:
   ```javascript
   async handlePaste(event) {
       const items = event.clipboardData?.items;
       // 检测图片类型
       if (item.type.indexOf('image') !== -1) {
           event.preventDefault();
           const file = item.getAsFile();
           // 上传图片...
       }
   }
   ```

3. **插入图片到编辑器**:
   ```javascript
   insertImageAtCursor(imageUrl) {
       const imageMarkdown = `![](${imageUrl})`;
       // 在光标位置插入...
   }
   ```

### 后端实现

在 `services/post_service.py` 中已经实现了完整的图片管理功能:

- `save_image()`: 保存上传的图片到文章的 `pics/` 目录
- `list_images()`: 列出文章目录下的所有图片

在 `app.py` 中的 API 端点:

- `POST /api/image/upload`: 上传图片
- `POST /api/image/list`: 列出图片

## 支持的图片格式

- PNG
- JPG/JPEG
- GIF
- SVG
- WebP

## 文件存储

- 图片保存在: `content/{文章目录}/pics/`
- 文件命名: `clipboard-{Unix时间戳}.png`
- 在 Markdown 中的引用: `![](pics/clipboard-xxxxx.png)`

## 注意事项

1. **必须先选择或创建文章**: 粘贴图片前需要打开一篇文章
2. **自动文件名**: 剪贴板粘贴的图片自动命名为 `clipboard-{timestamp}.png`
3. **即时预览**: 粘贴后立即在预览面板中显示
4. **文件大小**: 无限制,但建议优化图片大小以提升加载速度

## 用户体验优化

- ✅ 自动检测剪贴板内容
- ✅ 阻止默认粘贴行为(避免粘贴 base64 字符串)
- ✅ 显示上传进度提示
- ✅ 自动刷新图片列表
- ✅ 上传成功/失败通知
- ✅ 光标位置智能插入
