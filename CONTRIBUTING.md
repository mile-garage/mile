# Contributing to MILE

Thank you for helping! Bug reports, ideas and pull requests are welcome.

## Before you start

- For anything larger than a small fix, open an issue first to discuss it.
- Keep the style of the surrounding code; run `go vet ./...`, `go test ./...` and `npm --prefix web run typecheck` before sending a pull request.
- New user-facing text goes in `web/src/i18n.tsx`, in both Italian and English.

## License of contributions

MILE is licensed under the [AGPL-3.0-only](LICENSE). By contributing you agree that your contribution is licensed under the same terms, and you certify the [Developer Certificate of Origin](https://developercertificate.org/): you wrote it, or you have the right to submit it under this license.

Sign off each commit to confirm it:

```bash
git commit -s -m "Describe the change"
```

New source files start with:

```go
// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only
```

The MILE name and logo are covered by the [trademark policy](TRADEMARKS.md), not by the code license.
