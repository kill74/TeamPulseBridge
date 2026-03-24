# Profile Showcase Checklist

Use this checklist before sharing the repository with recruiters or hiring managers.

## 1. Visual Assets To Capture

- [ ] GitHub Actions overview page showing green `ci`, `smoke`, and `docs`
- [ ] Grafana dashboard with active metrics panels
- [ ] Terminal output of `make verify` passing
- [ ] API checks for `/healthz` and `/metrics`
- [ ] Docs site home page from GitHub Pages

## 2. Suggested Asset Filenames

Place screenshots/GIFs in `docs/media/`.

- `actions-green.png`
- `grafana-overview.png`
- `verify-pass.png`
- `metrics-endpoint.png`
- `docs-home.png`
- `demo-flow.gif`

## 3. README Integration Plan

After capturing assets, add a new section called "Demo Preview" to the README with:

- One CI screenshot
- One Grafana screenshot
- One short GIF showing local startup + endpoint checks

## 4. 3-Minute Demo Script

1. Show repository root and architecture section in README.
2. Show CI workflows and branch-protection expectations.
3. Run `make up` and hit `/healthz` and `/metrics`.
4. Open Grafana dashboard and explain one panel.
5. Show release workflow and changelog automation.

## 5. Presentation Tips

- Keep the story outcome-focused: reliability, security, observability.
- Emphasize engineering decisions and tradeoffs, not only tooling.
- Mention what you would scale next and why.
