// ingest stages public notes from the `notes` archive as Hugo page bundles.
//
// It is a thin orchestrator around the `notes` CLI: it calls
// `notes ls --public` for the ID list and `notes read --json` for the
// content, then writes one Hugo page bundle per note to
// <content>/<slug>/index.md.
//
// Two body transforms run before the markdown is written:
//
//  1. `[text](<id>)` references between public notes are rewritten to
//     `[text](/<slug>/)` using the ID→slug map built from the same fetch.
//  2. `![alt](../../images/<file>)` references — the convention used by the
//     notes archive after the cache migration — are copied from
//     $NOTES_PATH/images/<file> into the page bundle, and the markdown is
//     rewritten to a bare filename so Hugo's page-bundle resolution finds it.
//
// The tool has no dependency on the notes Go library and no knowledge of
// the npub image cache. Both are gone.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type noteRecord struct {
	ID          int      `json:"id"`
	UID         string   `json:"uid"`
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	Tags        []string `json:"tags"`
	Date        string   `json:"date"`
	Description string   `json:"description"`
	Public      bool     `json:"public"`
	Body        string   `json:"body"`
}

func main() {
	contentDir := flag.String("content", "content", "Hugo content directory to write to")
	flag.Parse()

	notesPath := os.Getenv("NOTES_PATH")
	if notesPath == "" {
		fatal("$NOTES_PATH is not set")
	}

	ids, err := listPublicIDs()
	if err != nil {
		fatal("notes ls --public: %v", err)
	}
	if len(ids) == 0 {
		fatal("no public notes returned by `notes ls --public`")
	}

	notes, err := readJSON(ids)
	if err != nil {
		fatal("notes read --json: %v", err)
	}

	// The CLI returns the raw frontmatter slug. Fall back to a slugified
	// title, then to the numeric ID — matching npub's chooseSlug() so that
	// notes without an explicit slug still get a stable URL.
	for i := range notes {
		notes[i].Slug = resolveSlug(notes[i])
	}

	// Build the ID → slug map across all public notes for link rewriting.
	idToSlug := make(map[int]string, len(notes))
	for _, n := range notes {
		idToSlug[n.ID] = n.Slug
	}

	if err := wipeBundles(*contentDir); err != nil {
		fatal("clearing %s: %v", *contentDir, err)
	}
	if err := os.MkdirAll(*contentDir, 0o755); err != nil {
		fatal("creating %s: %v", *contentDir, err)
	}

	imagesRoot := filepath.Join(notesPath, "images")
	for _, n := range notes {
		bundleDir := filepath.Join(*contentDir, n.Slug)
		if err := os.MkdirAll(bundleDir, 0o755); err != nil {
			fatal("creating bundle %s: %v", bundleDir, err)
		}

		body := rewriteNoteLinks(n.Body, idToSlug)
		body, attachments := flattenImagePaths(body)

		for _, name := range attachments {
			src := filepath.Join(imagesRoot, name)
			if err := copyFile(src, filepath.Join(bundleDir, name)); err != nil {
				warn("copying %s into %s: %v", src, bundleDir, err)
			}
		}

		md, err := renderHugoMarkdown(n, body)
		if err != nil {
			fatal("rendering %s: %v", n.UID, err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "index.md"), md, 0o644); err != nil {
			fatal("writing %s: %v", bundleDir, err)
		}
	}

	if err := writeHomeIndex(*contentDir); err != nil {
		fatal("writing home index: %v", err)
	}

	fmt.Fprintf(os.Stderr, "ingested %d public notes -> %s\n", len(notes), *contentDir)
}

func listPublicIDs() ([]string, error) {
	out, err := exec.Command("notes", "ls", "--public").Output()
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(out))
	return fields, nil
}

func readJSON(ids []string) ([]noteRecord, error) {
	args := append([]string{"read", "--json"}, ids...)
	out, err := exec.Command("notes", args...).Output()
	if err != nil {
		return nil, err
	}
	var notes []noteRecord
	if err := json.Unmarshal(out, &notes); err != nil {
		return nil, fmt.Errorf("decoding json: %w (first 200 bytes: %q)", err, truncate(out, 200))
	}
	// Sort newest first, matching npub's site order.
	sort.Slice(notes, func(i, j int) bool { return notes[i].UID > notes[j].UID })
	return notes, nil
}

func resolveSlug(n noteRecord) string {
	if s := slugify(n.Slug); s != "" {
		return s
	}
	if s := slugify(n.Title); s != "" {
		return s
	}
	return strconv.Itoa(n.ID)
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "`", "")
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// noteLinkPattern matches Markdown links whose destination is a positive
// integer (a note-ID reference).
var noteLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\((\d+)\)`)

func rewriteNoteLinks(body string, idToSlug map[int]string) string {
	return noteLinkPattern.ReplaceAllStringFunc(body, func(match string) string {
		m := noteLinkPattern.FindStringSubmatch(match)
		id, err := strconv.Atoi(m[2])
		if err != nil {
			return match
		}
		slug, ok := idToSlug[id]
		if !ok {
			return match
		}
		return "[" + m[1] + "](/" + slug + "/)"
	})
}

// imagePathPattern matches `![alt](../../images/X)` — the path convention
// used by the notes archive after the image cache migration.
var imagePathPattern = regexp.MustCompile(`!\[([^\]]*)\]\(\.\./\.\./images/([^)\s]+)\)`)

// flattenImagePaths rewrites `../../images/<file>` references to a bare
// filename (so they resolve against the Hugo page bundle), and returns
// the list of filenames that need to be copied into the bundle.
func flattenImagePaths(body string) (string, []string) {
	seen := map[string]struct{}{}
	var files []string
	out := imagePathPattern.ReplaceAllStringFunc(body, func(match string) string {
		m := imagePathPattern.FindStringSubmatch(match)
		alt, name := m[1], m[2]
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			files = append(files, name)
		}
		return "![" + alt + "](" + name + ")"
	})
	return out, files
}

func renderHugoMarkdown(n noteRecord, body string) ([]byte, error) {
	fm := map[string]any{
		"title":       n.Title,
		"date":        n.Date,
		"slug":        n.Slug,
		"description": n.Description,
		"uid":         n.UID,
		"aliases": []string{
			"/" + n.UID + "/",
			"/" + n.UID + "/" + n.Slug + "/",
		},
	}
	if len(n.Tags) > 0 {
		fm["tags"] = n.Tags
	}

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshalling frontmatter: %w", err)
	}

	var out strings.Builder
	out.WriteString("---\n")
	out.Write(yamlBytes)
	out.WriteString("---\n\n")
	out.WriteString(strings.TrimRight(body, "\n"))
	out.WriteString("\n")
	return []byte(out.String()), nil
}

// writeHomeIndex creates an empty content/_index.md if absent. Hugo needs
// it to render the home list page.
func writeHomeIndex(contentDir string) error {
	path := filepath.Join(contentDir, "_index.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("---\ntitle: \"\"\n---\n"), 0o644)
}

// wipeBundles removes every subdirectory of contentDir, leaving files
// (like _index.md) untouched. Subdirectories are owned by this tool: each
// one corresponds to a single note's page bundle.
func wipeBundles(contentDir string) error {
	entries, err := os.ReadDir(contentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := os.RemoveAll(filepath.Join(contentDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func truncate(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ingest: "+format+"\n", args...)
	os.Exit(1)
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ingest: warning: "+format+"\n", args...)
}
