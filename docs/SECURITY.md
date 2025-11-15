# Security Policy

## Supported Versions

We release patches for security vulnerabilities for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take the security of Hugo Admin seriously. If you believe you have found a security vulnerability, please report it to us as described below.

### Where to Report

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to: **svtter@qq.com**

You should receive a response within 48 hours. If for some reason you do not, please follow up via email to ensure we received your original message.

### What to Include

Please include the following information in your report:

- Type of issue (e.g., buffer overflow, SQL injection, cross-site scripting, etc.)
- Full paths of source file(s) related to the manifestation of the issue
- The location of the affected source code (tag/branch/commit or direct URL)
- Any special configuration required to reproduce the issue
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it

### What to Expect

- We will acknowledge receipt of your vulnerability report within 48 hours
- We will send a more detailed response within 7 days indicating the next steps
- We will work with you to understand and resolve the issue promptly
- We will keep you informed of the progress towards a fix
- We may ask for additional information or guidance

### Safe Harbor

We support safe harbor for security researchers who:

- Make a good faith effort to avoid privacy violations, destruction of data, and interruption or degradation of our services
- Only interact with accounts you own or with explicit permission of the account holder
- Do not exploit a security issue you discover for any reason (including demonstrating additional risk)
- Report vulnerabilities promptly
- Keep vulnerability details confidential until we've had a reasonable time to address them

We will not pursue legal action against researchers who follow these guidelines.

## Security Best Practices for Users

When using Hugo Admin, please follow these security best practices:

1. **Localhost Only**: By default, Hugo Admin binds to `127.0.0.1` (localhost only). Do not expose it to public networks without additional security measures.

2. **File Access**: Hugo Admin can only access files within the configured `CONTENT_DIR`. Ensure this directory only contains files you want to be editable.

3. **Authentication**: Consider adding authentication if you plan to run Hugo Admin on a network accessible by others.

4. **HTTPS**: If you must expose Hugo Admin over a network, use HTTPS with proper TLS certificates.

5. **Updates**: Keep Hugo Admin and its dependencies up to date to receive security patches.

6. **Environment Variables**: Store sensitive configuration in environment variables or `config_local.py` (which should be in `.gitignore`).

## Known Security Considerations

### Path Traversal Protection

Hugo Admin includes path traversal protection to prevent accessing files outside the configured content directory. However, users should still:

- Ensure `HUGO_ROOT` and `CONTENT_DIR` are correctly configured
- Avoid running Hugo Admin with elevated privileges

### WebSocket Connections

Hugo Admin uses WebSocket for real-time log streaming. These connections are not authenticated by default. Consider adding authentication if running on a shared network.

### Hugo Server Management

Hugo Admin can start and stop Hugo server processes. This capability should be restricted to trusted users only.

## Security Updates

Security updates will be released as soon as possible after a vulnerability is confirmed. Updates will be announced via:

- GitHub Security Advisories
- GitHub Releases
- Project README

## Contact

For security-related questions that are not vulnerabilities, please open a GitHub issue or contact the maintainers.

Thank you for helping keep Hugo Admin and its users safe!
