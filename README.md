# Notes theme for Hugo

A Hugo theme and supporting tooling that publishes a personal
[notes](https://github.com/dreikanter/notes) archive as a static site.

## Layout

- `hugo.yaml` — site configuration.
- `themes/notheme/` — the Hugo theme (layouts, partials, assets).
- `static/` — files copied verbatim into the deployed site root
  (`CNAME`, `wrangler.toml`, deploy-repo `README.md`).
- `tools/ingest/` — staging tool that reads the notes archive via the
  [`notes`](https://github.com/dreikanter/notes) CLI (`ls --public` +
  `read --json`) and writes Hugo page bundles into `content/<slug>/`.
- `tools/deploy.sh` — pushes `./public/` to the deploy git remote.
- `Makefile` — entry points.

## Usage

```sh
make ingest       # stage public notes from $NOTES_PATH into content/<slug>/
make build        # build the site into ./public/
make serve        # dev server on http://localhost:5000/
make deploy-dry   # build + commit locally, do not push
make deploy       # build + commit + push to alexmusayev.com.git
```

Override the deploy target with `DEPLOY_REPO=...` and `DEPLOY_BRANCH=...`.
