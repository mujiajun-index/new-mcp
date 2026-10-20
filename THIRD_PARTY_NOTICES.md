# Third-party notices

Third-party components retain their own copyright notices and license terms.
The project's `AGPL-3.0-or-later` declaration does not replace those notices.

## Adapted source: system-one-connector

- Component: `system-one-connector`
- Copyright: Copyright (c) 2026 Mostafa
- License: MIT
- Adapted files: `internal/mcp/systemone/client.go` and `internal/mcp/systemone/evaluate.go`
- Complete license: [internal/mcp/systemone/LICENSE](./internal/mcp/systemone/LICENSE)
- License location in the Docker image: `/app/licenses/system-one-connector/LICENSE`

Preserve the upstream copyright and complete MIT permission notice when
redistributing this code or substantial portions of it, including in binary form.

## Dependencies

This file records directly adapted source and is not an exhaustive dependency
license inventory. Go dependencies are recorded in `go.mod` and `go.sum`; frontend
dependencies are recorded in `web/package.json` and `web/package-lock.json`.
Their copyright and license notices remain in the respective module/package
distributions. The base image and installed runtime packages also have their own
licenses.

When preparing a binary or container release, retain the notices required by
the dependencies actually included in that release and satisfy their respective
license and source-distribution requirements. Any alternative license agreed for
project-owned code does not remove third-party license obligations.
