# GitHub Repository Setup Guide

This guide helps you configure your GitHub repository settings for optimal open source project presentation.

## Repository Settings

### 1. About Section

Go to your repository page and click the ⚙️ gear icon next to "About" to add:

**Description:**
```
🚀 A lightweight web-based admin interface for Hugo static sites - Browse, edit, and manage your blog with ease
```

**Website:**
```
https://github.com/sun-praise/hugo-admin
```

**Topics (click "Add topics"):**
```
hugo
hugo-admin
blog
cms
admin-panel
flask
python
markdown
static-site
web-interface
blog-management
hugo-theme
content-management
websocket
```

### 2. Repository Settings

Navigate to **Settings** → **General**:

#### Features
- ✅ Issues
- ✅ Discussions (Enable this for Q&A)
- ✅ Projects (optional)
- ✅ Wiki (optional)

#### Pull Requests
- ✅ Allow merge commits
- ✅ Allow squash merging
- ✅ Allow rebase merging
- ✅ Automatically delete head branches

### 3. Branch Protection (Optional but Recommended)

Navigate to **Settings** → **Branches** → **Add rule**:

**Branch name pattern:** `main`

Recommended settings:
- ✅ Require a pull request before merging
- ✅ Require status checks to pass before merging
  - Search and select: `test (3.9)`, `test (3.10)`, `test (3.11)`
- ✅ Require conversation resolution before merging
- ✅ Do not allow bypassing the above settings

### 4. Enable GitHub Discussions

Navigate to **Settings** → **General** → **Features**:
- ✅ Check "Discussions"

Then go to **Discussions** tab and create categories:
- 💬 General
- 💡 Ideas
- 🙏 Q&A
- 📣 Announcements
- 🐛 Bug Reports (redirect to Issues)

### 5. Security Settings

Navigate to **Settings** → **Security**:

#### Vulnerability Alerts
- ✅ Dependency graph
- ✅ Dependabot alerts
- ✅ Dependabot security updates

#### Code Security
- ✅ Secret scanning
- ✅ Push protection

### 6. Actions Permissions

Navigate to **Settings** → **Actions** → **General**:
- ✅ Allow all actions and reusable workflows
- Workflow permissions: **Read and write permissions**

## Using GitHub CLI (Alternative Method)

If you have GitHub CLI (`gh`) installed, you can set some of these programmatically:

```bash
# Set repository description and topics
gh repo edit sun-praise/hugo-admin \
  --description "🚀 A lightweight web-based admin interface for Hugo static sites - Browse, edit, and manage your blog with ease" \
  --add-topic hugo \
  --add-topic hugo-admin \
  --add-topic blog \
  --add-topic cms \
  --add-topic admin-panel \
  --add-topic flask \
  --add-topic python \
  --add-topic markdown \
  --add-topic static-site \
  --add-topic web-interface \
  --add-topic blog-management

# Enable features
gh repo edit sun-praise/hugo-admin \
  --enable-issues \
  --enable-wiki=false

# View current settings
gh repo view sun-praise/hugo-admin
```

## After Setup Checklist

- [ ] Repository description added
- [ ] Topics/tags added
- [ ] Issues enabled
- [ ] Discussions enabled (optional)
- [ ] Branch protection configured (optional)
- [ ] Dependabot enabled
- [ ] Check that GitHub Actions are running successfully
- [ ] Review README badges are displaying correctly
- [ ] Add repository to your profile (pin it if it's important)

## Sharing Your Project

Once setup is complete, consider:

1. **Write a blog post** about your project
2. **Share on social media** (Twitter, Reddit, etc.)
3. **Submit to directories**:
   - [Awesome Hugo](https://github.com/theNewDynamic/awesome-hugo)
   - [Hugo Themes](https://themes.gohugo.io/) (if applicable)
4. **Engage with Hugo community**:
   - [Hugo Discourse](https://discourse.gohugo.io/)
   - Hugo subreddit
5. **Create a demo video or GIF** showing the features

## Getting Stars

To attract contributors and users:
- Keep README updated with clear instructions
- Respond promptly to issues and PRs
- Add screenshots/demo GIFs
- Write good commit messages
- Tag releases properly
- Be welcoming to contributors

---

**Note:** This setup is complete! Your repository now has:
✅ Professional README with badges
✅ Security policy
✅ Code of conduct
✅ Contributing guidelines
✅ Issue templates
✅ PR template
✅ CI/CD workflow
