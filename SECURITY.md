# Security Policy

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in Zep Logger, please report it responsibly.

### How to Report

**Please do NOT create a public GitHub issue for security vulnerabilities.**

Instead, please report security issues by:
1. Emailing us at: [your-security-email@example.com]
2. Or creating a private security advisory on GitHub

### What to Include

When reporting a vulnerability, please include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fixes (optional)
- Your contact information

### Response Timeline

- We will acknowledge receipt within 48 hours
- We will provide a detailed response within 7 days
- We will keep you informed about our progress
- We will notify you when the vulnerability is fixed

### Disclosure Policy

- Please allow us reasonable time to fix the issue before public disclosure
- We will credit you in the security advisory (unless you prefer to remain anonymous)
- We will coordinate with you on the disclosure timeline

## Supported Versions

We provide security updates for:
- The latest release
- The previous major release (if applicable)

## Security Best Practices

When deploying Zep Logger:

1. **Protect Your Config File**
   - Never commit `config.toml` with real credentials
   - Use environment-specific configs
   - Rotate API keys regularly

2. **Network Security**
   - Use HTTPS for all external connections
   - Implement proper CORS policies
   - Consider rate limiting at the ingress layer

3. **Access Control**
   - Use least-privilege API keys
   - Implement authentication if exposing publicly
   - Monitor access logs

4. **Keep Updated**
   - Regularly update to the latest version
   - Monitor security advisories
   - Subscribe to release notifications

## Known Security Considerations

- This service accepts and forwards OTLP data - ensure your collector endpoint is trusted
- API keys are passed in Authorization headers - use TLS in production
- CORS is configurable - restrict to trusted origins only

## Contact

For security-related questions: [your-security-email@example.com]
