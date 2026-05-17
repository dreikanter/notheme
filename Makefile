.PHONY: ingest build serve clean all

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

# Remove generated content and build output.
clean:
  rm -rf content/notes public resources .hugo_build.lock

all: build
