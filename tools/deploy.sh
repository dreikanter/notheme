#!/usr/bin/env bash
#
# Publish ./public/ to the deploy git remote.
#
# Mirrors npub's deploy model: keep a bare clone of the deploy repo in
# ~/.cache/notheme/<repo>.git and use it as a git-dir against ./public/ as
# the work-tree. There is no second copy of the site on disk — git fetches
# the remote state into the bare clone, treats ./public/ as the live tree,
# and `add -A` reconciles add/modify/delete in one pass.
#
# Usage:
#   tools/deploy.sh [--dry-run]
#
# Env:
#   DEPLOY_REPO    git remote URL (default: git@github.com:dreikanter/alexmusayev.com.git)
#   DEPLOY_BRANCH  branch to push to (default: main)
#   CACHE_DIR      bare clone location (default: ~/.cache/notheme/<repo>.git)

set -euo pipefail

DEPLOY_REPO="${DEPLOY_REPO:-git@github.com:dreikanter/alexmusayev.com.git}"
DEPLOY_BRANCH="${DEPLOY_BRANCH:-main}"

repo_name=$(basename "$DEPLOY_REPO" .git)
CACHE_DIR="${CACHE_DIR:-$HOME/.cache/notheme/${repo_name}.git}"

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
fi

if [[ ! -d public ]]; then
  echo "deploy: ./public/ does not exist — run \`make build\` first." >&2
  exit 1
fi

if [[ ! -f public/index.html ]]; then
  echo "deploy: ./public/index.html missing — refusing to push an empty/half-built site." >&2
  exit 1
fi

mkdir -p "$(dirname "$CACHE_DIR")"

if [[ ! -d "$CACHE_DIR" ]]; then
  echo "deploy: creating bare clone at $CACHE_DIR"
  git clone --bare "$DEPLOY_REPO" "$CACHE_DIR"
else
  echo "deploy: fetching $DEPLOY_BRANCH from $DEPLOY_REPO"
  git --git-dir="$CACHE_DIR" fetch origin "$DEPLOY_BRANCH"
fi

# Point HEAD at the deploy branch so add/commit operate against the right ref.
git --git-dir="$CACHE_DIR" symbolic-ref HEAD "refs/heads/$DEPLOY_BRANCH"
git --git-dir="$CACHE_DIR" update-ref "refs/heads/$DEPLOY_BRANCH" \
  "refs/remotes/origin/$DEPLOY_BRANCH" 2>/dev/null || true

# Reset the index to match the remote so `add -A` sees true add/modify/delete.
GIT_INDEX_FILE="$CACHE_DIR/index" \
  git --git-dir="$CACHE_DIR" --work-tree="$PWD/public" \
  read-tree --reset -u "refs/heads/$DEPLOY_BRANCH" 2>/dev/null || true

# Stage the entire built site.
git --git-dir="$CACHE_DIR" --work-tree="$PWD/public" add -A

# Source commit (for traceability in the deploy log).
src_hash=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
src_clean=$(git diff-index --quiet HEAD -- 2>/dev/null && echo "" || echo " (dirty)")
msg="Site update from notheme@${src_hash}${src_clean}"

# Anything to commit?
if git --git-dir="$CACHE_DIR" --work-tree="$PWD/public" \
     diff --cached --quiet; then
  echo "deploy: nothing to commit — site already up to date."
  exit 0
fi

git --git-dir="$CACHE_DIR" --work-tree="$PWD/public" \
  -c user.name="notheme deploy" -c user.email="deploy@localhost" \
  commit -m "$msg"

if [[ $DRY_RUN -eq 1 ]]; then
  echo "deploy: --dry-run set, not pushing. Local commit:"
  git --git-dir="$CACHE_DIR" log --oneline -1
  exit 0
fi

echo "deploy: pushing to $DEPLOY_REPO ($DEPLOY_BRANCH)"
git --git-dir="$CACHE_DIR" push origin "$DEPLOY_BRANCH"
echo "deploy: done."
