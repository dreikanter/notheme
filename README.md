# Notes theme for Hugo

A Hugo theme + experimental configuration that renders the same site as
[npub](https://github.com/dreikanter/npub) from a local notes archive.

## Layout

- `hugo.yaml` — site config (site title, taxonomies, custom feed output,
  chroma highlighting, typographer disabled, `capitalizeListTitles: false`).
- `themes/notheme/` — the theme.
  - `layouts/_default/baseof.html` — head + inline CSS + nav + footer.
  - `layouts/_default/list.html` — home / section listing.
  - `layouts/_default/single.html` — note page with related-notes aside.
  - `layouts/_default/term.html` — `/tags/<tag>/` page.
  - `layouts/_default/home.feed.xml` — RSS feed at `/feed.xml`.
  - `layouts/_default/alias.html` — redirect page for `/<UID>/` and
    `/<UID>/<slug>/` legacy URLs.
  - `layouts/_default/_markup/render-codeblock.html` — code-block render hook
    that strips Hugo's extra wrappers so output matches npub's
    `<pre class="chroma"><code>…</code></pre>` exactly.
  - `assets/style.css` — copied verbatim from npub.
  - `assets/chroma.css` — chroma highlight CSS for both light and dark themes,
    scoped under `html:not(.dark)` / `html.dark`.
- `tools/ingest/` — small Go program that walks the notes archive via
  `github.com/dreikanter/notes` and stages every note with `public: true` as
  a Hugo page bundle under `content/notes/<slug>/index.md`. It rewrites
  `[text](<id>)` references between notes and copies cached external images.
  Configured by flags (`--notes`, `--assets`, `--content`); `--notes`
  defaults to `$NOTES_PATH`, matching the `notes` CLI convention.
- `Makefile` — `make ingest`, `make build`, `make serve`, `make clean`.

## How it stays in sync with the notes archive

The `notheme` repo never imports or modifies the notes. It reads them via the
`notes` Go library, the same library npub itself uses. So the slug rules,
public filter, UID derivation, and tag merging match npub exactly:

1. `make ingest` walks `$NOTES_PATH` (override with `--notes`) and stages
   every note whose frontmatter has `public: true`.
2. Each public note is rendered as a Hugo page bundle with frontmatter
   carrying `title`, `date`, `slug`, `tags`, `description`, `uid`, plus
   `aliases` for the legacy `/<UID>/` and `/<UID>/<slug>/` URLs.
3. Hugo's permalink config maps `notes/<slug>/index.md` to the URL `/<slug>/`,
   matching what npub produces.
4. Hugo's built-in alias generation produces the redirect HTML pages.

## Build and run locally

```sh
# Stage public notes from ~/Dropbox/Notes/ into content/notes/.
make ingest

# One-shot build to ./public/.
make build

# Live dev server on http://localhost:5000/ (rebuilds on note changes).
make serve
```

## Deploy

`make deploy` builds the site and pushes `./public/` to the deploy git remote
(default: `git@github.com:dreikanter/alexmusayev.com.git`, branch `main`).

The deploy is in `tools/deploy.sh` and mirrors npub's bare-clone model: it
keeps a bare clone at `~/.cache/notheme/<repo>.git` and uses `./public/` as
the work-tree, so there's no second copy of the site on disk and stale
files are removed automatically by `git add -A`.

```sh
make deploy-dry   # build + commit locally, do not push (inspect first)
make deploy       # build + commit + push
```

Override the target with env vars:

```sh
DEPLOY_REPO=git@github.com:user/staging.git DEPLOY_BRANCH=gh-pages make deploy
```

## Verifying against the live site

Tested with Playwright against both sites at the same viewport — visual
rendering is pixel-identical for the home, post, and tag pages.
