# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.4.x   | :white_check_mark: |
| 1.3.x   | :white_check_mark: |
| < 1.3   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability within SSG, please send an email to **spagu@github.com**. All security vulnerabilities will be promptly addressed.

**Please do not open a public GitHub issue for security vulnerabilities.**

### What to Include

When reporting a vulnerability, please include:

1. **Description**: A clear description of the vulnerability
2. **Steps to reproduce**: How can we reproduce the issue?
3. **Impact**: What is the potential impact?
4. **Version**: Which version of SSG is affected?

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Fix Release**: Within 30 days for critical issues

## Security Best Practices

### For Users

1. **Keep SSG Updated**: Always use the latest version
2. **Validate Input**: Sanitize content before processing
3. **Review Templates**: Audit custom templates for XSS vulnerabilities
4. **Use HTTPS**: Deploy generated sites over HTTPS
5. **Content Security Policy**: Configure appropriate CSP headers

### For Template Authors

1. **Escape Output**: Always escape user-provided content
2. **Avoid Inline JS**: Use external JavaScript files
3. **Validate URLs**: Check URLs before rendering links
4. **Sanitize HTML**: Use HTML sanitization for user content

### Built-in Security Features

SSG includes several security features:

- **Path Traversal Protection**: Prevents directory traversal attacks when extracting themes
- **Content Escaping**: HTML templates automatically escape content
- **Secure Defaults**: Safe configuration defaults
- **No Eval**: No dynamic code execution from templates

## Security Updates

Security updates are released as patch versions (e.g., 1.4.1, 1.4.2). Subscribe to releases on GitHub to stay informed.

## Acknowledgments

We appreciate security researchers who responsibly disclose vulnerabilities. Contributors will be acknowledged in release notes (unless they prefer to remain anonymous).
