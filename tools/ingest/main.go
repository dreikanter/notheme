// ingest reads public notes from a notes store and stages them as Hugo page
// bundles. It is the bridge between the npub-style notes archive and the
// Hugo theme in this repository.
//
// For each note with `public: true` in its frontmatter, ingest writes:
//
//	<content_dir>/notes/<slug>/index.md
//
// with Hugo-style frontmatter (title, date, slug, tags, aliases, description,
// uid). Body Markdown is preserved verbatim except for two transforms:
//
//  1. `[text](<id>)` references to other public notes are rewritten to
//     `[text](/<slug>/)` so Hugo doesn't need to know about note IDs.
//  2. External image URLs are resolved against the npub image cache
//     (~/Dropbox/Notes/images/index.json) — the cached file is copied into
//     the page bundle and the URL is rewritten to a relative filename.
//
// The slug computation, UID format, link rewriting, and legacy `/<UID>/` +
// `/<UID>/<slug>/` aliases all mirror what npub produces, so the resulting
// site URLs match what alexmusayev.com serves.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dreikanter/notes/note"
	"gopkg.in/yaml.v3"
)

type npubConfig struct {
	NotesPath  string `yaml:"notes_path"`
	AssetsPath string `yaml:"assets_path"`
	SiteName   string `yaml:"site_name"`
	AuthorName string `yaml:"author_name"`
	Intro      string `yaml:"intro"`
}

type imageEntry struct {
	FileName string `json:"file_name"`
	PageUID  string `json:"page_uid"`
}

func main() {
	configPath := flag.String("config", expandHome("~/Dropbox/Notes/npub.yml"), "path to npub.yml (provides notes_path and image cache location)")
	contentDir := flag.String("content", "content", "Hugo content directory to write to")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fatal("loading config %s: %v", *configPath, err)
	}
	if cfg.NotesPath == "" {
		fatal("notes_path is empty in %s", *configPath)
	}
	if cfg.AssetsPath == "" {
		cfg.AssetsPath = filepath.Join(cfg.NotesPath, "images")
	}

	store := note.NewOSStore(cfg.NotesPath)
	entries, err := store.All(note.WithPublic(true))
	if err != nil {
		fatal("reading notes: %v", err)
	}

	pages := buildPages(entries)
	idToSlug := make(map[int]string, len(pages))
	for _, p := range pages {
		idToSlug[p.ID] = p.Slug
	}

	imageCache, err := loadImageCache(cfg.AssetsPath)
	if err != nil {
		warn("loading image cache: %v", err)
		imageCache = make(map[string]imageEntry)
	}

	notesOut := filepath.Join(*contentDir, "notes")
	if err := wipeContent(notesOut); err != nil {
		fatal("clearing %s: %v", notesOut, err)
	}
	if err := os.MkdirAll(notesOut, 0o755); err != nil {
		fatal("creating %s: %v", notesOut, err)
	}

	for _, p := range pages {
		bundleDir := filepath.Join(notesOut, p.Slug)
		if err := os.MkdirAll(bundleDir, 0o755); err != nil {
			fatal("creating bundle %s: %v", bundleDir, err)
		}

		body := rewriteNoteLinks(p.Body, idToSlug)
		body, attachments := rewriteImages(body, p.UID, cfg.AssetsPath, imageCache)

		for src, dst := range attachments {
			if err := copyFile(src, filepath.Join(bundleDir, dst)); err != nil {
				warn("copying attachment %s -> %s: %v", src, dst, err)
			}
		}

		md, err := renderHugoMarkdown(p, body)
		if err != nil {
			fatal("rendering %s: %v", p.UID, err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "index.md"), md, 0o644); err != nil {
			fatal("writing %s: %v", bundleDir, err)
		}
	}

	if err := writeHomeIndex(*contentDir); err != nil {
		fatal("writing home index: %v", err)
	}

	fmt.Fprintf(os.Stderr, "ingested %d public notes -> %s\n", len(pages), notesOut)
}

type pageInfo struct {
	ID          int
	UID         string
	Slug        string
	Title       string
	Description string
	Tags        []string
	Date        string // YYYY-MM-DDTHH:MM:SS+ZONE
	Body        string
}

func buildPages(entries []note.Entry) []pageInfo {
	out := make([]pageInfo, 0, len(entries))
	for _, e := range entries {
		uid := e.Meta.CreatedAt.Format("20060102") + "_" + strconv.Itoa(e.ID)
		slug := chooseSlug(e)
		title := e.Meta.Title
		if title == "" {
			title = uid
		}
		out = append(out, pageInfo{
			ID:          e.ID,
			UID:         uid,
			Slug:        slug,
			Title:       title,
			Description: e.Meta.Description,
			Tags:        e.Meta.Tags,
			Date:        e.Meta.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Body:        e.Body,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID > out[j].UID })
	return out
}

func chooseSlug(e note.Entry) string {
	if s := slugify(e.Meta.Slug); s != "" {
		return s
	}
	if s := slugify(e.Meta.Title); s != "" {
		return s
	}
	return strconv.Itoa(e.ID)
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "`", "")
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// linkPattern captures Markdown links whose href is a positive integer (note
// ID reference). Group 1 = link text, group 2 = id.
var linkPattern = regexp.MustCompile(`\[([^\]]+)\]\((\d+)\)`)

func rewriteNoteLinks(body string, idToSlug map[int]string) string {
	return linkPattern.ReplaceAllStringFunc(body, func(match string) string {
		m := linkPattern.FindStringSubmatch(match)
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

// imagePattern: ![alt](url) — URL only when it starts with http(s)://
var imagePattern = regexp.MustCompile(`!\[([^\]]*)\]\((https?://[^)\s]+)\)`)

func rewriteImages(body, uid, assetsRoot string, cache map[string]imageEntry) (string, map[string]string) {
	attachments := make(map[string]string)
	out := imagePattern.ReplaceAllStringFunc(body, func(match string) string {
		m := imagePattern.FindStringSubmatch(match)
		alt, url := m[1], m[2]
		entry, ok := cache[url]
		if !ok {
			return match
		}
		src := filepath.Join(assetsRoot, entry.PageUID, entry.FileName)
		if _, err := os.Stat(src); err != nil {
			warn("image not on disk for %s in note %s (expected %s)", url, uid, src)
			return match
		}
		attachments[src] = entry.FileName
		return "![" + alt + "](" + entry.FileName + ")"
	})
	return out, attachments
}

func renderHugoMarkdown(p pageInfo, body string) ([]byte, error) {
	fm := map[string]any{
		"title":       p.Title,
		"date":        p.Date,
		"slug":        p.Slug,
		"description": p.Description,
		"uid":         p.UID,
		"type":        "notes",
		"aliases": []string{
			"/" + p.UID + "/",
			"/" + p.UID + "/" + p.Slug + "/",
		},
	}
	if len(p.Tags) > 0 {
		fm["tags"] = p.Tags
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

func writeHomeIndex(contentDir string) error {
	path := filepath.Join(contentDir, "_index.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	body := "---\ntitle: \"\"\n---\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func loadConfig(path string) (npubConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return npubConfig{}, err
	}
	var cfg npubConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return npubConfig{}, err
	}
	cfg.NotesPath = expandHome(cfg.NotesPath)
	cfg.AssetsPath = expandHome(cfg.AssetsPath)
	return cfg, nil
}

func loadImageCache(assetsPath string) (map[string]imageEntry, error) {
	data, err := os.ReadFile(filepath.Join(assetsPath, "index.json"))
	if err != nil {
		return nil, err
	}
	var idx map[string]imageEntry
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return idx, nil
}

func wipeContent(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(dir)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ingest: "+format+"\n", args...)
	os.Exit(1)
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ingest: warning: "+format+"\n", args...)
}
