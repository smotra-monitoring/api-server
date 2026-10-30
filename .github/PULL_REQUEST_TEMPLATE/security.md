<!--
Prefer GitHub private vulnerability reporting for undisclosed vulnerabilities, with the same four items below.
Open a public PR only after a maintainer has agreed. See CONTRIBUTING.md.
Security PRs missing any of the four required sections are closed without review.
-->

## 1. Vulnerability explanation

- Affected component and versions:
- Vulnerability class (e.g. CWE / OWASP category):
- Root cause:
- Impact:
- Attacker prerequisites:
- Severity assessment:

## 2. Steps to reproduce

<!-- Exact, minimal, numbered steps from a clean checkout, including requests or payloads. -->

1.
2.
3.

Expected (secure) behaviour:

Actual (vulnerable) behaviour:

## 3. Unit test reproducing the vulnerability

Test file and name: `<path>_test.go` / `Test...`

- [ ] The test fails on the current code without the fix
- [ ] The test passes with the fix applied

## 4. Proposed fix or mitigation

<!-- Describe the change. If a full fix is not possible, describe the mitigation and its limitations. -->

- [ ] The fix is included in this PR
- [ ] The fix does not change unrelated behaviour

## Verification

- [ ] `just fmt`, `just lint` and `just test` pass locally
- [ ] Documentation updated if behaviour changed

## Author declaration

- [ ] I have read CONTRIBUTING.md
- [ ] I understand every change in this PR and can explain it in review
- [ ] I have signed the CLA

AI assistance used (tool and extent, or "none"):
