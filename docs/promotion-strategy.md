# wgpilot Promotion Strategy

> Goal: Get first users. Stage: Pre-launch / early launch.
> Audience: Self-hosters, small teams, MSPs/IT admins.

---

## Phase 1: Launch Preparation (Before You Post Anywhere)

Do these first. Every link you share will drive people to your GitHub repo, and you get one shot at a first impression.

### 1.1 Screenshot the Dashboard

Take 3-5 high-quality screenshots and save them to `docs/assets/`:

- **Dashboard overview** — peer status cards, transfer chart, live indicators
- **Network creation** — showing topology mode selector
- **Peer config modal** — with QR code visible
- **Setup wizard** — the clean 4-step flow
- **Mobile view** — if responsive, show it

Put the best screenshot at the top of the README (uncomment the placeholder line). This is the single most impactful thing you can do. People scroll GitHub READMEs for 5 seconds — a screenshot stops them.

### 1.2 Record a 60-Second GIF/Video

Record a terminal + browser side-by-side showing:

1. `curl` install (3 seconds)
2. `wgpilot init` (2 seconds)
3. `wgpilot serve` (2 seconds)
4. Open browser, complete setup wizard (15 seconds)
5. Create a network, add a peer, scan QR code (30 seconds)
6. Show peer coming online in real-time (10 seconds)

Tools: [asciinema](https://asciinema.org/) for terminal, OBS for browser, or [VHS](https://github.com/charmbracelet/vhs) for scripted terminal recordings.

Embed this GIF in the README right below the header. This is your "demo" and it's worth more than 1000 words of documentation.

### 1.3 Create a GitHub Release

- Tag `v0.2.0` with a proper changelog
- Upload the `wgpilot_linux_amd64` binary as a release asset
- Write release notes highlighting the top 5 features
- This makes the install `curl` command actually work

### 1.4 Set Up GitHub Repository

- Add topics: `wireguard`, `vpn`, `self-hosted`, `golang`, `network-management`, `wireguard-manager`, `web-ui`
- Write a one-line description: "Self-hosted WireGuard management with web UI. Single binary, kernel-native, no shell-outs."
- Add a social preview image (1280x640 PNG) — use the dashboard screenshot with the logo overlaid
- Enable Discussions tab for community Q&A
- Create issue templates (bug report, feature request)

---

## Phase 2: Launch Posts (Week 1)

### 2.1 Reddit — Primary Channel

Reddit is where your target audience lives. Post to these subreddits **one at a time** over 2-3 days (not all at once — that looks spammy):

#### r/selfhosted (780k+ members) — YOUR #1 TARGET
```
Title: I built wgpilot — a single-binary WireGuard manager with web UI,
       multi-network support, and no shell-outs

Body:
Hey r/selfhosted,

I've been building wgpilot, a self-hosted WireGuard management tool.
The main difference from existing tools (wg-easy, Firezone, etc.) is that
it uses kernel APIs directly instead of shelling out to wg/ip/iptables,
and ships as a single Go binary with everything embedded.

What it does:
- Single binary: Go server + React UI + SQLite + TLS — no Docker required
- Multi-network: 3 topology modes (VPN gateway, site-to-site, hub-routed)
- Network bridging: Connect separate WG networks with nftables rules
- Real-time dashboard: Live peer status via SSE, transfer charts
- Security: Encrypted private keys (AES-256-GCM), RBAC, audit log, API keys
- Setup: curl install → init → browser wizard → done

Install:
  curl -fsSL <release-url> -o /usr/local/bin/wgpilot
  wgpilot init && wgpilot serve

It also runs in Docker if you prefer that.

[Screenshot of dashboard]

GitHub: https://github.com/itsChris/wgpilot

I'd love feedback on what's missing or what would make this useful for
your setup. Happy to answer questions!
```

**Timing**: Post Tuesday-Thursday, 9-11 AM EST (peak r/selfhosted traffic).

#### r/WireGuard (90k+ members)
Focus on the **technical differentiator**: kernel-native management, no shell-outs, reconciliation on startup. This audience is more technical and cares about correctness.

#### r/homelab (2M+ members)
Angle: "One more tool for the stack." Show how it fits into a homelab setup (Proxmox VM, bare metal, Docker).

#### r/golang (200k+ members)
Angle: Architecture and engineering. Talk about the no-shell-out approach, wgctrl-go, pure-Go SQLite, go:embed for the SPA. Go developers love clean architecture posts.

#### r/linux (900k+ members)
Angle: Native Linux tool, systemd integration, nftables, kernel API. Emphasis on "no Docker required."

### 2.2 Hacker News

Post as "Show HN":

```
Title: Show HN: wgpilot – Single-binary WireGuard manager with web UI (kernel-native, no shell-outs)
URL: https://github.com/itsChris/wgpilot
```

HN tips:
- Post between 8-10 AM EST on a weekday (Tuesday-Thursday best)
- Be in the comments immediately to answer questions
- Lead with the technical story: "Most WG tools shell out to wg and parse text. I wanted to use the kernel API directly."
- Be honest about limitations and what's next
- Don't be salesy — HN hates that
- If it doesn't get traction the first time, you can repost after a week

### 2.3 Lobsters

If you have an account (invite-only), post there too. Same angle as HN but smaller, more focused audience.

---

## Phase 3: Social Media (Week 1-2)

### 3.1 Twitter/X

Thread format works best:

```
Tweet 1 (hook):
I built a WireGuard management tool that doesn't shell out to wg, ip,
or iptables. Instead, it talks to the Linux kernel directly via netlink.

Single binary. Embedded web UI. SQLite. No Docker required.

Here's wgpilot: [link] [screenshot]

Tweet 2:
Most WG tools work like this:
  exec("wg show") → parse text → exec("wg set") → hope it worked

wgpilot uses:
  wgctrl-go → kernel netlink socket → typed responses

No parsing. No race conditions. No missing error codes.

Tweet 3:
It supports 3 network topologies:
- VPN Gateway (remote access with NAT)
- Site-to-Site (connect office LANs)
- Hub-Routed (team mesh, peers reach each other)

Plus network bridging — connect wg0 and wg1 with controlled forwarding.

Tweet 4:
Security isn't bolted on:
- Private keys encrypted at rest (AES-256-GCM)
- Multi-user with RBAC (admin/viewer)
- Audit log on every mutation
- API keys for automation
- Rate-limited login

Tweet 5:
Get started in 60 seconds:
  curl ... -o /usr/local/bin/wgpilot
  wgpilot init
  wgpilot serve
  → Open browser → 4-step wizard → done

MIT licensed. Feedback welcome.
GitHub: [link]
```

Tag: `#WireGuard #VPN #SelfHosted #Golang #Linux #OpenSource`

### 3.2 LinkedIn

Write a post aimed at IT professionals and DevOps engineers:

```
I just open-sourced wgpilot — a WireGuard management tool built for
teams and IT administrators.

What makes it different:
→ Single binary deployment (no Docker, no PostgreSQL, no Redis)
→ Talks to the Linux kernel directly (no shell-outs to wg/ip/iptables)
→ Multi-network with 3 topology modes + network bridging
→ Enterprise features: RBAC, audit log, API keys, encrypted key storage

If you manage WireGuard for your team or clients, I'd love your feedback.

GitHub: [link]
```

### 3.3 Mastodon / BlueSky

Shorter posts, link to the GitHub. Focus on the self-hosted angle for Mastodon (that community values self-sovereignty).

---

## Phase 4: Content Marketing (Week 2-4)

### 4.1 Blog Post: "Why I Built wgpilot"

Write a technical blog post (host on dev.to, Medium, or your own blog). Structure:

1. **The problem**: Managing WireGuard at scale is painful. Existing tools have limitations.
2. **The approach**: Kernel-native via wgctrl-go/netlink/nftables. Why this matters (reliability, error handling, speed).
3. **Architecture deep dive**: Single binary, embedded SPA, SQLite as source of truth, startup reconciliation.
4. **What I learned**: Challenges with netlink API, nftables in Go, embedding React in Go.
5. **What's next**: Roadmap, how to contribute.

Cross-post to: dev.to, Medium, Hashnode, lobste.rs, r/golang, r/programming.

### 4.2 Blog Post: "WireGuard Topologies Explained"

Educational content that naturally showcases wgpilot:

1. VPN Gateway — when and why
2. Site-to-Site — connecting offices
3. Hub-Routed — team mesh
4. Network Bridging — advanced multi-network setups

Use the mermaid diagrams from the README. This is evergreen content that will rank in search.

### 4.3 YouTube Video: "Set Up a Multi-Network WireGuard VPN in 5 Minutes"

Even a simple screen recording with voiceover would work:

1. Install wgpilot (30s)
2. Run setup wizard (60s)
3. Create two networks (gateway + hub-routed) (90s)
4. Add peers, scan QR on phone (60s)
5. Show bridging between networks (60s)

Upload to YouTube with good SEO:
- Title: "WireGuard Management Made Easy — wgpilot Setup Tutorial"
- Tags: wireguard, vpn, self-hosted, linux, wireguard setup, wireguard ui
- Description: Link to GitHub, timestamps, feature list

---

## Phase 5: Community Building (Ongoing)

### 5.1 Be Present Where Users Are

- **Answer WireGuard questions** on r/WireGuard, r/selfhosted, StackOverflow, ServerFault
- When relevant, mention wgpilot as a solution (don't spam — be genuinely helpful first)
- Monitor GitHub issues and respond quickly (< 24 hours)
- Enable GitHub Discussions for Q&A

### 5.2 Integrations and Ecosystem

- **Awesome lists**: Submit to [awesome-selfhosted](https://github.com/awesome-selfhosted/awesome-selfhosted) (under VPN), [awesome-wireguard](https://github.com/cedrickchee/awesome-wireguard)
- **Alternative.to**: Create a listing
- **Docker Hub**: Publish the image with good documentation
- **Ansible role / Terraform module**: Makes adoption easier for ops teams

### 5.3 Comparison Content

Create `docs/comparison.md` or a website page that honestly compares wgpilot to:
- wg-easy (simpler but limited)
- Firezone (more complex, requires PostgreSQL)
- Netbird (mesh-focused, different use case)
- Tailscale (hosted, not self-hosted)
- Headscale (WireGuard-based but Tailscale-compatible)

Be fair. Acknowledge when alternatives are better for specific use cases. People respect honesty.

---

## Phase 6: Amplification (Month 2+)

### 6.1 Reach Out to Content Creators

Contact YouTubers and bloggers who cover self-hosted tools:

- **Techno Tim** — self-hosted infrastructure
- **NetworkChuck** — networking tutorials (large audience)
- **Jeff Geerling** — Linux and homelab
- **Christian Lempa** — self-hosted tools
- **Lawrence Systems** — networking and firewalls
- **DB Tech** — self-hosted Docker tutorials
- **Awesome Open Source** — open source showcases

Approach: "Hey, I built this open-source WireGuard management tool. Would you be interested in checking it out? Happy to set up a demo instance for you."

### 6.2 Conference Talks

Submit to:
- **GopherCon** — "Building a Kernel-Native Network Manager in Go"
- **FOSDEM** — Networking / Go devrooms
- **All Things Open** — Open source tools
- Local Linux User Groups (LUGs) — lightning talks

---

## Launch Checklist

Before posting anywhere, verify:

- [ ] README has a screenshot/GIF at the top
- [ ] GitHub release exists with downloadable binary
- [ ] Install curl command actually works end-to-end
- [ ] Docker image is published and pullable
- [ ] Repository has topics, description, and social preview
- [ ] You've tested the full flow: install → init → serve → setup wizard → create network → add peer → connect
- [ ] GitHub Discussions or Issues are enabled for feedback
- [ ] You have time to respond to comments for 48 hours after each post

---

## Messaging Cheat Sheet

Use these angles depending on the audience:

| Audience | Lead With |
|----------|-----------|
| Self-hosters | "Single binary, no Docker required, curl install" |
| DevOps/SREs | "API keys, Prometheus metrics, audit log, systemd integration" |
| Security-minded | "No shell-outs, encrypted keys, RBAC, rate limiting" |
| Go developers | "wgctrl-go, pure-Go SQLite, kernel netlink API, clean architecture" |
| MSPs/IT admins | "Multi-network, 3 topologies, network bridging, multi-user" |
| WireGuard users | "Kernel-native management, startup reconciliation, config export/import" |

---

## Key Differentiators to Always Emphasize

1. **No shell-outs** — this is unique and technically impressive
2. **Single binary** — radically simpler deployment than alternatives
3. **Multi-network + bridging** — most tools only handle one network
4. **Encrypted private keys** — most tools store keys in plaintext
5. **SQLite as source of truth + reconciliation** — robust state management
