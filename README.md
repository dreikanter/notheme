# Notes theme for Hugo

A Hugo theme and supporting tooling that publishes a personal
[notes](https://github.com/dreikanter/notes) archive as a static site.

## Layout

- `hugo.yaml` — site configuration.
- `themes/notheme/` — the Hugo theme (layouts, partials, assets).
- `tools/ingest/` — staging tool that reads the notes archive and writes
  Hugo page bundles into `content/notes/`. The long-term goal is to
  interact with the archive strictly through the `notes` CLI; today the
  tool still links the `notes` Go library while the CLI catches up
  ([dreikanter/notes#282](https://github.com/dreikanter/notes/issues/282),
  [#283](https://github.com/dreikanter/notes/issues/283)).
- `tools/deploy.sh` — pushes `./public/` to the deploy git remote.
- `Makefile` — entry points.

## Usage

```sh
make ingest       # stage public notes from $NOTES_PATH into content/notes/
make build        # build the site into ./public/
make serve        # dev server on http://localhost:5000/
make deploy-dry   # build + commit locally, do not push
make deploy       # build + commit + push to alexmusayev.com.git
```

Override the deploy target with `DEPLOY_REPO=...` and `DEPLOY_BRANCH=...`.
