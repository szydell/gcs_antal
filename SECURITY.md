# Security Policy
## Reporting a Vulnerability
The GCS Antal team takes security issues very seriously. We appreciate your efforts to responsibly disclose your findings and will make every effort to acknowledge your contributions.
### Reporting Process
To report a security vulnerability in GCS Antal, please follow these steps:
1. **DO NOT** disclose the vulnerability publicly in the issue tracker, as this could put all users at risk.
2. Submit your report privately through our GitHub Security Advisory system: [https://github.com/szydell/gcs_antal/security/advisories/new](https://github.com/szydell/gcs_antal/security/advisories/new)
3. Provide as much information as possible, including:
    - A detailed description of the vulnerability
    - Steps to reproduce the issue
    - Potential impact of the vulnerability
    - Suggested mitigation or remediation steps (if any)
    - Your contact information for follow-up questions

### What to Expect
After submitting a security vulnerability report:
1. **Acknowledgment**: We will acknowledge your report within 48 hours.
2. **Verification**: Our team will work to verify the reported issue.
3. **Remediation**: If the vulnerability is confirmed, we will develop and test a fix.
4. **Disclosure**: We will coordinate with you on an appropriate disclosure timeline, typically after a fix has been deployed.
5. **Credit**: With your permission, we'll acknowledge your contribution when we publish the security advisory.

## Supported Versions
We provide security updates for the latest minor release of GCS Antal. We encourage all users to keep their installations up to date.
## Security Best Practices
When deploying GCS Antal, consider these security best practices:
1. **Keep Updated**: Always use the latest version of GCS Antal to benefit from security patches.
2. **Limit Access**: Restrict network access to GCS Antal to only necessary services.
3. **Use Secure Connections**: Configure TLS for all connections between GCS Antal, NATS servers, and GitLab.
4. **Minimum Privileges**: For GitLab tokens used with GCS Antal, follow the principle of least privilege.
5. **Monitor Logs**: Regularly review logs for suspicious activity.

## Security Advisories
When security issues are reported, verified, and fixed, we will publish security advisories in the [Security Advisories](https://github.com/szydell/gcs_antal/security/advisories) section of our GitHub repository.
## Thank You
We greatly appreciate the security research community's efforts in helping ensure GCS Antal remains secure. Your contributions make our project safer for everyone.
