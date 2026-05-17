.PHONY: ingest build serve deploy deploy-dry clean all

# Ingest public notes from ~/Dropbox/Notes and stage them under content/notes/.
# Run the Go tool from its module dir but write content to the project root.
ingest:
	cd tools/ingest && go run . --content ../../content

# Build the Hugo site to ./public/.
build: ingest
	hugo --cleanDestinationDir --minify=false

# Run Hugo's dev server on http://localhost:5000/.
serve: ingest
	hugo server --bind=127.0.0.1 --port=5000 --disableFastRender

# Publish ./public/ to the deploy remote (defaults to alexmusayev.com.git).
# Override with DEPLOY_REPO / DEPLOY_BRANCH env vars.
deploy: build
	./tools/deploy.sh

# Same as deploy but stops before `git push` so you can inspect the commit.
deploy-dry: build
	./tools/deploy.sh --dry-run

# Remove generated content and build output.
clean:
	rm -rf content/notes public resources .hugo_build.lock

all: build
