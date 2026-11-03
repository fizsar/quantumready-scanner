# Security policy

QuantumReady is a personal project, run by one person. Reports are read and answered on a best-effort basis; there is no bug bounty.

## Reporting a vulnerability

Write to **soporte@fizsar.com** (English or Spanish), as listed in our [security.txt](https://quantumready.fizsar.com/.well-known/security.txt). Please do not open a public issue for a vulnerability.

Useful to include: what is affected (this code, or the service at quantumready.fizsar.com), how to reproduce it, and what an attacker could do with it.

## Scope

- This repository's code.
- The service at https://quantumready.fizsar.com.

Out of scope: denial of service and volume tests against the production service, social engineering, and findings that only affect a third party's website (QuantumReady reports them about the site you scan; it does not fix them).

## Testing the production service

Use the service as any visitor would. Please do not run automated scanners or load tests against it, and do not use it to probe hosts you are not allowed to test: every scan comes from our server's address and is subject to the limits described in [/about-scans](https://quantumready.fizsar.com/about-scans). To try the scanner against weak servers, use the lab in [test/tlslab](test/tlslab), which runs on an internal Docker network.

## Excluding your domain from scans

That is not a vulnerability report: see [/about-scans](https://quantumready.fizsar.com/about-scans), or write to the same address.
