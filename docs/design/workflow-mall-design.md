# Mol Mall Design

> **Status: Vision document** — Phase 1 (local templates) exists. Phases 2-5 (registry, publishing, federation) are not implemented.

> A marketplace for Gas Town templates

## Vision

**Mol Mall** is a registry for sharing templates across Gas Town installations. Think npm for workflows, or Terraform Registry for workflows.

```
"Cook a template, dispatch it to a worker, the watcher watches, merger merges."

What if you could browse a mall of templates, install one, and immediately
have your workers executing world-class workflows?
```

### The Network Effect

A well-designed template for "code review" or "security audit" or "deploy to K8s" can spread across thousands of Gas Town installations. Each adoption means:
- More agents executing proven workflows
- More structured, trackable work output
- Better capability routing (agents with track records on a template get similar work)

## Architecture

### Registry Types

```
┌─────────────────────────────────────────────────────────────────┐
│                      MOL MALL REGISTRIES                         │
└─────────────────────────────────────────────────────────────────┘

PUBLIC REGISTRY (molmall.gastown.io)
├── Community templates (MIT licensed)
├── Official Gas Town templates (blessed)
├── Verified publisher templates
└── Open contribution model

PRIVATE REGISTRY (self-hosted)
├── Organization-specific templates
├── Proprietary workflows
├── Internal deployment patterns
└── Enterprise compliance templates

FEDERATED REGISTRY (HOP future)
├── Cross-organization discovery
├── Skill-based search
└── Attribution chain tracking
└── hop:// URI resolution
```

### URI Scheme

```
hop://molmall.gastown.io/templates/wf-worker-work@4.0.0
       └──────────────────┘         └──────────────┘ └───┘
           registry host              template name   version

# Short forms
wf-worker-work                    # Default registry, latest version
wf-worker-work@4                  # Major version
wf-worker-work@4.0.0              # Exact version
@acme/wf-deploy                    # Scoped to publisher
hop://acme.corp/templates/wf-deploy # Full HOP URI
```

### Registry API

```yaml
# OpenAPI-style specification

GET /templates
  # List all templates
  Query:
    - q: string          # Search query
    - capabilities: string[]   # Filter by capability tags
    - author: string     # Filter by author
    - limit: int
    - offset: int
  Response:
    templates:
      - name: wf-worker-work
        version: 4.0.0
        description: "Full worker work lifecycle..."
        author: steve@gastown.io
        downloads: 12543
        capabilities: [go, testing, code-review]

GET /templates/{name}
  # Get template metadata
  Response:
    name: wf-worker-work
    versions: [4.0.0, 3.2.1, 3.2.0, ...]
    latest: 4.0.0
    author: steve@gastown.io
    repository: https://github.com/steveyegge/gastown
    license: MIT
    capabilities:
      primary: [go, testing]
      secondary: [git, code-review]
    stats:
      downloads: 12543
      stars: 234
      used_by: 89  # workspaces using this template

GET /templates/{name}/{version}
  # Get specific version
  Response:
    name: wf-worker-work
    version: 4.0.0
    checksum: sha256:abc123...
    signature: <optional PGP signature>
    content: <base64 or URL to .template.toml>
    changelog: "Added self-cleaning model..."
    published_at: 2026-01-10T00:00:00Z

POST /templates
  # Publish template (authenticated)
  Body:
    name: wf-my-workflow
    version: 1.0.0
    content: <template TOML>
    changelog: "Initial release"
  Auth: Bearer token (linked to HOP identity)

GET /templates/{name}/{version}/download
  # Download template content
  Response: raw .template.toml content
```

## Template Package Format

### Simple Case: Single File

Most templates are single `.template.toml` files:

```bash
gt template install wf-worker-code-review
# Downloads wf-worker-code-review.template.toml to ~/gt/.beads/templates/
```

### Complex Case: Template Bundle

Some templates need supporting files (scripts, templates, configs):

```
wf-deploy-k8s.template.bundle/
├── template.toml              # Main template
├── templates/
│   ├── deployment.yaml.tmpl
│   └── service.yaml.tmpl
├── scripts/
│   └── healthcheck.sh
└── README.md
```

Bundle format:
```bash
# Bundles are tarballs
wf-deploy-k8s-1.0.0.bundle.tar.gz
```

Installation:
```bash
gt template install wf-deploy-k8s
# Extracts to ~/gt/.beads/templates/wf-deploy-k8s/
# template.toml is at wf-deploy-k8s/template.toml
```

## Installation Flow

### Basic Install

```bash
$ gt template install wf-worker-code-review

Resolving wf-worker-code-review...
  Registry: molmall.gastown.io
  Version:  1.2.0 (latest)
  Author:   steve@gastown.io
  Skills:   code-review, security

Downloading... ████████████████████ 100%
Verifying checksum... ✓

Installed to: ~/gt/.beads/templates/wf-worker-code-review.template.toml
```

### Version Pinning

```bash
$ gt template install wf-worker-work@4.0.0

Installing wf-worker-work@4.0.0 (pinned)...
✓ Installed

$ gt template list --installed
  wf-worker-work           4.0.0   [pinned]
  wf-worker-code-review    1.2.0   [latest]
```

### Upgrade Flow

```bash
$ gt template upgrade wf-worker-code-review

Checking for updates...
  Current: 1.2.0
  Latest:  1.3.0

Changelog for 1.3.0:
  - Added security focus option
  - Improved test coverage step

Upgrade? [y/N] y

Downloading... ✓
Installed: wf-worker-code-review@1.3.0
```

### Lock File

```json
// ~/gt/.beads/templates/.lock.json
{
  "version": 1,
  "templates": {
    "wf-worker-work": {
      "version": "4.0.0",
      "pinned": true,
      "checksum": "sha256:abc123...",
      "installed_at": "2026-01-10T00:00:00Z",
      "source": "hop://molmall.gastown.io/templates/wf-worker-work@4.0.0"
    },
    "wf-worker-code-review": {
      "version": "1.3.0",
      "pinned": false,
      "checksum": "sha256:def456...",
      "installed_at": "2026-01-10T12:00:00Z",
      "source": "hop://molmall.gastown.io/templates/wf-worker-code-review@1.3.0"
    }
  }
}
```

## Publishing Flow

### First-Time Setup

```bash
$ gt template publish --init

Setting up Mol Mall publishing...

1. Create account at https://molmall.gastown.io/signup
2. Generate API token at https://molmall.gastown.io/settings/tokens
3. Run: gt template login

$ gt template login
Token: ********
Logged in as: steve@gastown.io
```

### Publishing

```bash
$ gt template publish wf-worker-work

Publishing wf-worker-work...

Pre-flight checks:
  ✓ template.toml is valid
  ✓ Version 4.0.0 not yet published
  ✓ Required fields present (name, version, description)
  ✓ Skills declared

Publish to molmall.gastown.io? [y/N] y

Uploading... ✓
Published: hop://molmall.gastown.io/templates/wf-worker-work@4.0.0

View at: https://molmall.gastown.io/templates/wf-worker-work
```

### Verification Levels

```
┌─────────────────────────────────────────────────────────────────┐
│                    FORMULA TRUST LEVELS                          │
└─────────────────────────────────────────────────────────────────┘

UNVERIFIED (default)
  Anyone can publish
  Basic validation only
  Displayed with ⚠️ warning

VERIFIED PUBLISHER
  Publisher identity confirmed
  Displayed with ✓ checkmark
  Higher search ranking

OFFICIAL
  Maintained by Gas Town team
  Displayed with 🏛️ badge
  Included in embedded defaults

AUDITED
  Security review completed
  Displayed with 🔒 badge
  Required for enterprise registries
```

## Capability Tagging

### Template Capability Declaration

```toml
[template.capabilities]
# What capabilities does this template exercise? Used for agent routing.
primary = ["go", "testing", "code-review"]
secondary = ["git", "ci-cd"]

# Capability weights (optional, for fine-grained routing)
[template.capabilities.weights]
go = 0.3           # 30% of template work is Go
testing = 0.4      # 40% is testing
code-review = 0.3  # 30% is code review
```

### Capability-Based Search

```bash
$ gt template search --capabilities="security,go"

Templates matching capabilities: security, go

  wf-security-audit           v2.1.0   ⭐ 4.8   📥 8,234
    Capabilities: security, go, code-review
    "Comprehensive security audit workflow"

  wf-dependency-scan          v1.0.0   ⭐ 4.2   📥 3,102
    Capabilities: security, go, supply-chain
    "Scan Go dependencies for vulnerabilities"
```

### Agent Accountability

When a worker completes a template, the execution is tracked:

```
Worker: beads/amber
Template: wf-worker-code-review@1.3.0
Completed: 2026-01-10T15:30:00Z
Capabilities exercised:
  - code-review (primary)
  - security (secondary)
  - go (secondary)
```

This execution record enables:
1. **Routing** - Agents with successful track records get similar work
2. **Debugging** - Trace which agent did what, when
3. **Quality metrics** - Track success rates by agent and template

## Private Registries

### Enterprise Deployment

```yaml
# ~/.gtconfig.yaml
registries:
  - name: acme
    url: https://molmall.acme.corp
    auth: token
    priority: 1  # Check first

  - name: public
    url: https://molmall.gastown.io
    auth: none
    priority: 2  # Fallback
```

### Self-Hosted Registry

```bash
# Docker deployment
docker run -d \
  -p 8080:8080 \
  -v /data/templates:/templates \
  -e AUTH_PROVIDER=oidc \
  gastown/molmall-registry:latest

# Configuration
MOLMALL_STORAGE=s3://bucket/templates
MOLMALL_AUTH=oidc
MOLMALL_OIDC_ISSUER=https://auth.acme.corp
```

## Federation

Federation enables template sharing across organizations using the Highway Operations Protocol (HOP).

### Cross-Registry Discovery

```bash
$ gt template search "deploy kubernetes" --federated

Searching across federated registries...

  molmall.gastown.io:
    wf-deploy-k8s           v3.0.0   🏛️ Official

  molmall.acme.corp:
    @acme/wf-deploy-k8s     v2.1.0   ✓ Verified

  molmall.bigco.io:
    @bigco/k8s-workflow      v1.0.0   ⚠️ Unverified
```

### HOP URI Resolution

The `hop://` URI scheme provides cross-registry entity references:

```bash
# Full HOP URI
gt template install hop://molmall.acme.corp/templates/@acme/wf-deploy@2.1.0

# Resolution via HOP (Highway Operations Protocol)
1. Parse hop:// URI
2. Resolve registry endpoint (DNS/HOP discovery)
3. Authenticate (if required)
4. Download template
5. Verify checksum/signature
6. Install to workspace-level
```

## Implementation Phases

### Phase 1: Local Commands (Now)

See [Template Resolution](template-resolution.md) for the implemented three-tier resolution system.

### Phase 2: Manual Sharing

- Template export/import
- `gt template export wf-worker-work > wf-worker-work.template.toml`
- `gt template import < wf-worker-work.template.toml`
- Lock file format

### Phase 3: Public Registry

- molmall.gastown.io launch
- `gt template install` from registry
- `gt template publish` flow
- Basic search and browse

### Phase 4: Enterprise Features

- Private registry support
- Authentication integration
- Verification levels
- Audit logging

### Phase 5: Federation (HOP)

- Capability tags in schema
- Federation protocol (Highway Operations Protocol)
- Cross-registry search
- Agent execution tracking for accountability

## Related Documents

- [Template Resolution](template-resolution.md) - Local resolution order
- [Workflows](../concepts/workflows.md) - Template lifecycle (cook, pour, squash)
