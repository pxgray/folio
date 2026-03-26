package web

import (
	"errors"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/render"
)

type docData struct {
	Title       string
	Content     template.HTML
	Breadcrumbs []breadcrumb
	RepoBase    string
	RepoName    string
	Ref         string
}

type dirData struct {
	Title       string
	Entries     []gitstore.TreeEntry
	IndexHTML   template.HTML
	Breadcrumbs []breadcrumb
	RepoBase    string
	RepoName    string
	Ref         string
	// currentPath is the dir path within the repo, used to build entry URLs.
	currentPath string
}

// EntryURL builds the URL for a directory entry (called from dir.html).
func (d dirData) EntryURL(name string, isDir bool) string {
	var base string
	if d.currentPath == "" {
		base = d.RepoBase + "/" + name
	} else {
		base = d.RepoBase + "/" + d.currentPath + "/" + name
	}
	if d.Ref != "" {
		return base + "?ref=" + d.Ref
	}
	return base
}

func (s *Server) handleDoc(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	// chi wildcard is stored as "*"
	filePath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	ref := r.URL.Query().Get("ref")

	// Normalise trailing slash: redirect to clean URL.
	if len(r.URL.Path) > 1 && strings.HasSuffix(r.URL.Path, "/") {
		clean := strings.TrimRight(r.URL.Path, "/")
		if ref != "" {
			clean += "?ref=" + ref
		}
		http.Redirect(w, r, clean, http.StatusMovedPermanently)
		return
	}

	gr, err := s.store.Get(host, owner, repo)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotRegistered) {
			httpError(w, http.StatusNotFound, "repo not found: "+host+"/"+owner+"/"+repo)
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	hash, err := gr.ResolveRef(r.Context(), ref)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotFound) {
			httpError(w, http.StatusNotFound, "ref not found: "+ref)
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	repoBase := "/" + host + "/" + owner + "/" + repo
	repoName := host + "/" + owner + "/" + repo

	if filePath == "" {
		s.serveDirPage(w, gr, hash, repoBase, repoName, "", ref)
		return
	}

	// Try reading as a blob first.
	blob, err := gr.ReadBlob(hash, filePath)
	if err == nil {
		if strings.HasSuffix(filePath, ".md") {
			s.serveMarkdownPage(w, blob, repoBase, repoName, filePath, ref)
			return
		}
		rawURL := repoBase + "/-/raw/" + filePath + refQuery(ref)
		http.Redirect(w, r, rawURL, http.StatusFound)
		return
	}
	if !errors.Is(err, gitstore.ErrNotFound) {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Try as a directory.
	_, treeErr := gr.ReadTree(hash, filePath)
	if treeErr == nil {
		s.serveDirPage(w, gr, hash, repoBase, repoName, filePath, ref)
		return
	}

	httpError(w, http.StatusNotFound, "not found: "+filePath)
}

func (s *Server) serveMarkdownPage(w http.ResponseWriter, src []byte, repoBase, repoName, filePath, ref string) {
	content, err := render.Render(src, repoBase, filePath, ref)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "render error: "+err.Error())
		return
	}
	title := headingTitle(string(src))
	if title == "" {
		title = filePath
	}
	data := docData{
		Title:       title,
		Content:     content,
		Breadcrumbs: buildBreadcrumbs(repoBase, filePath, ref),
		RepoBase:    repoBase,
		RepoName:    repoName,
		Ref:         ref,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(w, "doc.html", data)
}

func (s *Server) serveDirPage(w http.ResponseWriter, gr *gitstore.Repo, hash plumbing.Hash, repoBase, repoName, dirPath, ref string) {
	entries, err := gr.ReadTree(hash, dirPath)
	if err != nil {
		httpError(w, http.StatusNotFound, "not found: "+dirPath)
		return
	}

	// Sort: directories first, then alphabetically.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	// Render index.md if present.
	var indexHTML template.HTML
	for _, e := range entries {
		if e.Name == "index.md" && !e.IsDir {
			var indexPath string
			if dirPath == "" {
				indexPath = "index.md"
			} else {
				indexPath = dirPath + "/index.md"
			}
			src, err := gr.ReadBlob(hash, indexPath)
			if err == nil {
				indexHTML, _ = render.Render(src, repoBase, indexPath, ref)
			}
			break
		}
	}

	title := repoName
	if dirPath != "" {
		title = dirPath
	}

	data := dirData{
		Title:       title,
		Entries:     entries,
		IndexHTML:   indexHTML,
		Breadcrumbs: buildBreadcrumbs(repoBase, dirPath, ref),
		RepoBase:    repoBase,
		RepoName:    repoName,
		Ref:         ref,
		currentPath: dirPath,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(w, "dir.html", data)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title       string
		Repos       interface{}
		RepoBase    string
		RepoName    string
		Breadcrumbs []breadcrumb
	}{
		Title: "Folio",
		Repos: s.store.Repos(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(w, "index.html", data)
}

// headingTitle extracts the text of the first # heading from Markdown source.
func headingTitle(src string) string {
	for _, line := range strings.SplitN(src, "\n", 20) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
