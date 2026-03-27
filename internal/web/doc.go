package web

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/nav"
	"github.com/pxgray/folio/internal/render"
)

type docData struct {
	Title       string
	Content     template.HTML
	TOC         template.HTML
	Breadcrumbs []breadcrumb
	RepoBase    string
	RepoName    string
	Ref         string
	Nav         []nav.Item
	CurrentPath string
}

type dirData struct {
	Title       string
	Entries     []gitstore.TreeEntry
	TOC         template.HTML
	Breadcrumbs []breadcrumb
	RepoBase    string
	RepoName    string
	Ref         string
	Nav         []nav.Item
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
	filePath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	ref := r.URL.Query().Get("ref")

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

	repoBase := "/" + host + "/" + owner + "/" + repo
	repoName := host + "/" + owner + "/" + repo
	s.serveRepo(w, r, gr, ref, repoBase, repoName, filePath, true)
}

func (s *Server) handleLocalDoc(w http.ResponseWriter, r *http.Request) {
	label := chi.URLParam(r, "label")
	filePath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")

	if len(r.URL.Path) > 1 && strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, strings.TrimRight(r.URL.Path, "/"), http.StatusMovedPermanently)
		return
	}

	gr, err := s.store.GetLocal(label)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotRegistered) {
			httpError(w, http.StatusNotFound, "local repo not found: "+label)
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	repoBase := "/local/" + label
	repoName := "local/" + label
	s.serveRepo(w, r, gr, "", repoBase, repoName, filePath, false)
}

// serveRepo resolves the ref, loads navigation, and routes to a markdown page,
// directory page, or raw redirect. If allowRaw is false, non-.md files return 404.
func (s *Server) serveRepo(w http.ResponseWriter, r *http.Request, repo gitstore.Repository, ref, repoBase, repoName, filePath string, allowRaw bool) {
	hash, err := repo.ResolveRef(r.Context(), ref)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotFound) {
			httpError(w, http.StatusNotFound, "ref not found: "+ref)
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	navItems := loadNav(repo, hash)
	if !navCoversPath(navItems, filePath) {
		navItems = nil
	}

	if filePath == "" {
		s.serveDirPage(w, repo, hash, repoBase, repoName, "", ref, navItems)
		return
	}

	blob, err := repo.ReadBlob(hash, filePath)
	if err == nil {
		if strings.HasSuffix(filePath, ".md") {
			s.serveMarkdownPage(w, blob, repoBase, repoName, filePath, ref, navItems)
			return
		}
		if allowRaw {
			rawURL := repoBase + "/-/raw/" + filePath + refQuery(ref)
			http.Redirect(w, r, rawURL, http.StatusFound)
			return
		}
		httpError(w, http.StatusNotFound, "not found: "+filePath)
		return
	}
	if !errors.Is(err, gitstore.ErrNotFound) {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, treeErr := repo.ReadTree(hash, filePath)
	if treeErr == nil {
		s.serveDirPage(w, repo, hash, repoBase, repoName, filePath, ref, navItems)
		return
	}

	httpError(w, http.StatusNotFound, "not found: "+filePath)
}

func (s *Server) serveMarkdownPage(w http.ResponseWriter, src []byte, repoBase, repoName, filePath, ref string, navItems []nav.Item) {
	result, err := render.Render(src, repoBase, filePath, ref)
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
		Content:     result.Content,
		TOC:         result.TOC,
		Breadcrumbs: buildBreadcrumbs(repoBase, filePath, ref),
		RepoBase:    repoBase,
		RepoName:    repoName,
		Ref:         ref,
		Nav:         navItems,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.docTmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("folio: render doc %s: %v", filePath, err)
	}
}

func (s *Server) serveDirPage(w http.ResponseWriter, repo gitstore.Repository, hash plumbing.Hash, repoBase, repoName, dirPath, ref string, navItems []nav.Item) {
	entries, err := repo.ReadTree(hash, dirPath)
	if err != nil {
		httpError(w, http.StatusNotFound, "not found: "+dirPath)
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	// If index.md exists, render it as a doc page instead of the directory listing.
	for _, e := range entries {
		if e.Name == "index.md" && !e.IsDir {
			indexPath := "index.md"
			if dirPath != "" {
				indexPath = dirPath + "/index.md"
			}
			if src, err := repo.ReadBlob(hash, indexPath); err == nil {
				s.serveMarkdownPage(w, src, repoBase, repoName, indexPath, ref, navItems)
				return
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
		Breadcrumbs: buildBreadcrumbs(repoBase, dirPath, ref),
		RepoBase:    repoBase,
		RepoName:    repoName,
		Ref:         ref,
		Nav:         navItems,
		currentPath: dirPath,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.dirTmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("folio: render dir %s: %v", dirPath, err)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title       string
		Repos       interface{}
		Locals      interface{}
		TOC         template.HTML
		RepoBase    string
		RepoName    string
		Breadcrumbs []breadcrumb
		Nav         []nav.Item
	}{
		Title:  "Folio",
		Repos:  s.store.Repos(),
		Locals: s.store.Locals(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.indexTmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("folio: render index: %v", err)
	}
}

// navCoversPath reports whether filePath is explicitly covered by the nav items —
// either as an exact leaf match or as a directory that contains at least one leaf.
func navCoversPath(items []nav.Item, filePath string) bool {
	for _, item := range items {
		if item.Path != "" {
			if item.Path == filePath {
				return true
			}
			// filePath is a directory ancestor of this leaf.
			if filePath != "" && strings.HasPrefix(item.Path, filePath+"/") {
				return true
			}
		}
		if navCoversPath(item.Children, filePath) {
			return true
		}
	}
	return false
}

// loadNav loads navigation items for the repo. It first tries to read folio.yml
// from the repo root; if absent or unparseable, it falls back to auto-generating
// nav from the directory tree.
func loadNav(repo gitstore.Repository, hash plumbing.Hash) []nav.Item {
	if data, err := repo.ReadBlob(hash, "folio.yml"); err == nil {
		if _, items, err := nav.Parse(data); err == nil {
			return items
		}
	}
	walker := func(dirPath string) ([]nav.WalkEntry, error) {
		entries, err := repo.ReadTree(hash, dirPath)
		if err != nil {
			return nil, err
		}
		result := make([]nav.WalkEntry, len(entries))
		for i, e := range entries {
			result[i] = nav.WalkEntry{Name: e.Name, IsDir: e.IsDir}
		}
		return result, nil
	}
	return nav.AutoGenerate(walker, "")
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
