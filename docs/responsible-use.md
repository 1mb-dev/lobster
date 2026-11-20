---
title: Responsible Use
nav_order: 4
---

# Responsible Use

Only run Lobster against sites you own or have written permission to test. Without permission, a crawl or a load can break computer-misuse laws and the site's terms of service. When in doubt, get permission in writing.

## What Lobster Enforces

- **robots.txt**, including `Crawl-delay`, checked again on every redirect. Use `-ignore-robots` only with the site owner's permission.
- **Rate**: `-rate` is the total for all workers, at least 0.1 requests per second. A 429 response backs the crawl off; during `-load`, 10 consecutive 429, 5xx or connection errors stop the load.
- **Addresses**: every connection's address is checked. Localhost and private addresses need `-allow-private-ips`, and redirects stay on the base URL's origin.
- **Load ownership**: a `-load` against a public host, or one whose robots.txt sets a `Crawl-delay`, needs `-i-own-this` and `LOBSTER_I_OWN_THIS=<host>`. A load without them can connect only to private addresses.
- **TLS** is verified. Skipping verification needs both `-insecure-skip-verify` and `LOBSTER_INSECURE_TLS=true`.
- **Credentials** come from environment variables or stdin, never from flag values.

## Before a Load

- Crawl first, without `-load`: the crawl shows how many pages a load would replay and how the site responds.
- Start with a short load at a low rate, then raise one at a time:

  ```bash
  lobster -url http://localhost:3000 -allow-private-ips -rate 10 -load 30s
  ```

- Tell the people who run the host, and watch its metrics while the load runs. A long `-load` nobody watches needs their approval.
- Prefer a staging host. Do not rerun an aborted load at the same rate.

## Credentials

Use a dedicated test account with only the access the crawled pages need, never a real user's.

```bash
LOBSTER_AUTH_TOKEN="$TEST_TOKEN" lobster -url http://localhost:3000 -allow-private-ips -auth-type bearer
```

## Reports

The JSON and HTML reports list every URL crawled, query string included, and the errors seen. Lobster writes them readable by your user only (mode 0600). Do not commit them, and redact them before sharing.
