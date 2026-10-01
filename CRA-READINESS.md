# CRA Readiness

This project voluntarily documents its security practices using the Security Slam
[CRA Readiness checklist](https://securityslam.com/library/cra-readiness), aligned
with the EU [Cyber Resilience Act](https://openssf.org/public-policy/eu-cyber-resilience-act/).

## Disclaimer

> _This project voluntarily documents its security practices._
> _This information is provided "as is", without warranties or guarantees._
> _The maintainers and contributors:_
>
> - _have no obligations under the EU CRA,_
> - _are not Manufacturers, Importers, or Economic Operators,_
> - _assume no financial, contractual, or legal liability,_
> - _and do not provide CRA compliance assurances._
>
> _Entities incorporating this software into commercial products remain solely responsible for regulatory compliance, risk assessment, and vulnerability management._

For more context, see the ORC WG [maintainer transparency FAQ](https://cra.orcwg.org/faq/maintainers/transparency/).

## Checklist

Last reviewed: 2026-10-01

| Item | Description | Link to artifact |
| --- | --- | --- |
| Cybersecurity and Vulnerability Management Policy | Covers secure development practices, risk handling, security contact, the vulnerability reporting, remediation, and disclosure process, and the support period and end-of-life process. | [Security Policy](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md): [Secure Development](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md#secure-development), [Risk Handling](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md#risk-handling), [Security Contact](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md#security-contact), [Vulnerability Process](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md#vulnerability-process), [End of Life](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md#end-of-life) |
| Contributing Guidance | Contributing guide links to secure development practices. | [Contributing Guidelines](https://github.com/privateerproj/.github/blob/main/.github/CONTRIBUTING.md) |
| Release Documentation | Release notes describe new functionality and security fixes. | [Releases](https://github.com/privateerproj/privateer-sdk/releases) |
| Bug Reporting Guide | Process for reporting non-security bugs, separate from security reporting. | [Issue Report Process](https://github.com/privateerproj/.github/blob/main/.github/CONTRIBUTING.md#issue-report-process) and the [bug report template](https://github.com/privateerproj/privateer-sdk/issues/new?template=bug_report.yml). Security reports go through the [Security Policy](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md#report-a-vulnerability) instead. |
| MFA Enforcement | MFA is enabled for all contributors and required for admins. | The `privateerproj` GitHub organization requires two-factor authentication for all members and outside collaborators. |
| Branch Protection | Branch protection is enabled on the default branch. | An active repository ruleset on `main` blocks deletion and force pushes, and requires a pull request with one approval, code owner review, resolved review threads, and passing status checks. |
| License File | The repository contains a clear, OSI-approved license. | [Apache License 2.0](https://github.com/privateerproj/privateer-sdk/blob/main/LICENSE) |
| OSPS Baseline | The project meets OSPS Baseline Level 1 or higher. | [OpenSSF Best Practices Baseline Level 1](https://www.bestpractices.dev/projects/12018/baseline-1) and the weekly [OSPS Security Assessment](https://github.com/privateerproj/privateer-sdk/actions/workflows/osps-security-assessment.yml) scan |
