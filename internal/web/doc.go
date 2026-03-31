package web

import (
	"bytes"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
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
	CurrentPath string
}

// EntryURL builds the URL for a directory entry (called from dir.html).
func (d dirData) EntryURL(name string, isDir bool) string {
	var base string
	if d.CurrentPath == "" {
		base = d.RepoBase + "/" + name
	} else {
		base = d.RepoBase + "/" + d.CurrentPath + "/" + name
	}
	if d.Ref != "" {
		return base + "?ref=" + url.QueryEscape(d.Ref)
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
			clean += "?ref=" + url.QueryEscape(ref)
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

	key := host + "/" + owner + "/" + repo
	trusted := s.repoTrusted[key]

	repoBase := "/" + host + "/" + owner + "/" + repo
	repoName := host + "/" + owner + "/" + repo
	s.serveRepo(w, r, gr, ref, repoBase, repoName, filePath, true, trusted)
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

	trusted := s.localTrusted[label]

	repoBase := "/local/" + label
	repoName := "local/" + label
	s.serveRepo(w, r, gr, "", repoBase, repoName, filePath, false, trusted)
}

// serveRepo resolves the ref, loads navigation, and routes to a markdown page,
// directory page, or raw redirect. If allowRaw is false, non-.md files return 404.
func (s *Server) serveRepo(w http.ResponseWriter, r *http.Request, repo gitstore.Repository, ref, repoBase, repoName, filePath string, allowRaw bool, trusted bool) {
	hash, err := s.resolveRef(w, repo, ref)
	if err != nil {
		return
	}

	navItems := s.loadNavAndCheck(repo, hash, filePath)

	if filePath == "" {
		s.serveDirPage(w, repo, hash, repoBase, repoName, "", ref, navItems, trusted)
		return
	}

	s.dispatchToContent(w, r, repo, hash, repoBase, repoName, filePath, ref, navItems, trusted, allowRaw)
}

// resolveRef resolves the ref and returns the hash, or writes an error response.
func (s *Server) resolveRef(w http.ResponseWriter, repo gitstore.Repository, ref string) (plumbing.Hash, error) {
	hash, err := repo.ResolveRef(nil, ref)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotFound) {
			httpError(w, http.StatusNotFound, "ref not found: "+ref)
			return plumbing.ZeroHash, err
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return plumbing.ZeroHash, err
	}
	return hash, nil
}

// loadNavAndCheck loads nav items and filters them to only include paths that cover filePath.
func (s *Server) loadNavAndCheck(repo gitstore.Repository, hash plumbing.Hash, filePath string) []nav.Item {
	navItems := loadNav(repo, hash)
	if !navCoversPath(navItems, filePath) {
		navItems = nil
	}
	return navItems
}

// dispatchToContent routes to a markdown page, directory page, or raw redirect.
func (s *Server) dispatchToContent(w http.ResponseWriter, r *http.Request, repo gitstore.Repository, hash plumbing.Hash, repoBase, repoName, filePath, ref string, navItems []nav.Item, trusted bool, allowRaw bool) {
	blob, err := repo.ReadBlob(hash, filePath)
	if err == nil {
		if strings.HasSuffix(filePath, ".md") {
			s.serveMarkdownPage(w, blob, repoBase, repoName, filePath, ref, navItems, trusted)
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
		s.serveDirPage(w, repo, hash, repoBase, repoName, filePath, ref, navItems, trusted)
		return
	}

	httpError(w, http.StatusNotFound, "not found: "+filePath)
}

func (s *Server) serveMarkdownPage(w http.ResponseWriter, src []byte, repoBase, repoName, filePath, ref string, navItems []nav.Item, trusted bool) {
	result, err := render.Render(src, repoBase, filePath, ref, trusted)
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
		CurrentPath: filePath,
	}

	var buf bytes.Buffer
	if err := s.docTmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("folio: render doc %s: %v", filePath, err)
		httpError(w, http.StatusInternalServerError, "template error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) serveDirPage(w http.ResponseWriter, repo gitstore.Repository, hash plumbing.Hash, repoBase, repoName, dirPath, ref string, navItems []nav.Item, trusted bool) {
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
				s.serveMarkdownPage(w, src, repoBase, repoName, indexPath, ref, navItems, trusted)
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
		CurrentPath: dirPath,
	}

	var buf bytes.Buffer
	if err := s.dirTmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("folio: render dir %s: %v", dirPath, err)
		httpError(w, http.StatusInternalServerError, "template error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
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

	var buf bytes.Buffer
	if err := s.indexTmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("folio: render index: %v", err)
		httpError(w, http.StatusInternalServerError, "template error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
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
	for _, line := range strings.SplitN(src, "\n", 100) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
