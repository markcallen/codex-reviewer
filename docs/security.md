# Security and Secret Handling

This repository is intended to be public. Treat every committed file and every
commit in Git history as public.

## Local secrets

- Keep real credentials in an ignored `.env` file or in your system secret
  manager.
- Start from `.env.example` when you need local environment variables.
- Do not commit `.env`, `.env.*`, Codex auth JSON, GitHub tokens, OpenAI API
  keys, kubeconfigs, SSH keys, signing keys, or registry credentials.
- Prefer short-lived or least-privilege tokens for local testing.

## Before publishing or tagging a release

Run:

```bash
make secrets-scan
```

The target uses `gitleaks detect` to scan committed Git history. It intentionally
does not scan ignored local files such as `.env`, because those can contain real
developer credentials that should never appear in command output.

To inspect the full local checkout, including ignored files, run this only on a
trusted machine and keep the report private:

```bash
gitleaks dir . --redact --no-banner
```

## If a secret is found

1. Revoke or rotate the credential immediately.
2. Remove the secret from the working tree.
3. If the secret was committed, rewrite history before publishing the repository
   or coordinate a public-history cleanup if it has already been pushed.
4. Rerun `make secrets-scan` and confirm no leaks remain.
