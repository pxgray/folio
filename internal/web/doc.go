package web

import (
	"bytes"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/nav"
	"github.com/pxgray/folio/internal/render"
)

type docData struct {
	Title         string
	Content       template.HTML
	TOC           template.HTML
	Breadcrumbs   []breadcrumb
	RepoBase      string
	RepoName      string
	Ref           string
	Nav           []nav.Item
	CurrentPath   string
	Sections      []nav.Section
	ActiveSection int
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
	s.mu.RLock()
	trusted := s.repoTrusted[key]
	s.mu.RUnlock()

	repoBase := "/" + host + "/" + owner + "/" + repo
	repoName := host + "/" + owner + "/" + repo
	s.serveRepo(w, r, gr, ref, repoBase, repoName, filePath, trusted)
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

	trusted := false // local repos: TrustedHTML not yet supported via db.Store

	repoBase := "/local/" + label
	repoName := "local/" + label
	s.serveRepo(w, r, gr, "", repoBase, repoName, filePath, trusted)
}

func (s *Server) serveRepo(w http.ResponseWriter, r *http.Request, repo gitstore.Repository, ref, repoBase, repoName, filePath string, trusted bool) {
	hash, err := s.resolveRef(w, repo, ref)
	if err != nil {
		return
	}

	if filePath == "" {
		navResult := loadNavResult(repo, hash)
		navItems := sectionsToNav(navResult.Sections)
		if leaf := firstNavLeaf(navItems); leaf != "" {
			dest := repoBase + "/" + leaf + refQuery(ref)
			http.Redirect(w, r, dest, http.StatusFound)
			return
		}
		s.serveRepoRoot(w, repo, hash, repoBase, repoName, ref, navResult, trusted)
		return
	}

	navResult := s.loadNavAndCheck(repo, hash, filePath)
	s.dispatchToContent(w, r, repo, hash, repoBase, repoName, filePath, ref, navResult, trusted)
}

func sectionsToNav(sections []nav.Section) []nav.Item {
	if len(sections) == 0 {
		return nil
	}
	var all []nav.Item
	for _, s := range sections {
		all = append(all, s.Nav...)
	}
	return all
}

func (s *Server) serveRepoRoot(w http.ResponseWriter, repo gitstore.Repository, hash plumbing.Hash, repoBase, repoName, ref string, navResult nav.ParseResult, trusted bool) {
	src, err := repo.ReadBlob(hash, "index.md")
	if err == nil {
		s.serveMarkdownPage(w, src, repoBase, repoName, "index.md", ref, navResult, trusted)
		return
	}
	if !errors.Is(err, gitstore.ErrNotFound) {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpError(w, http.StatusNotFound, "not found")
}

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

func (s *Server) loadNavAndCheck(repo gitstore.Repository, hash plumbing.Hash, filePath string) nav.ParseResult {
	navResult := loadNavResult(repo, hash)
	navItems := sectionsToNav(navResult.Sections)
	if !navCoversPath(navItems, filePath) {
		navResult.Sections = nil
	}
	return navResult
}

func (s *Server) dispatchToContent(w http.ResponseWriter, r *http.Request, repo gitstore.Repository, hash plumbing.Hash, repoBase, repoName, filePath, ref string, navResult nav.ParseResult, trusted bool) {
	blob, err := repo.ReadBlob(hash, filePath)
	if err == nil {
		if strings.HasSuffix(filePath, ".md") {
			s.serveMarkdownPage(w, blob, repoBase, repoName, filePath, ref, navResult, trusted)
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
		indexBlob, indexErr := repo.ReadBlob(hash, filePath+"/index.md")
		if indexErr == nil {
			s.serveMarkdownPage(w, indexBlob, repoBase, repoName, filePath+"/index.md", ref, navResult, trusted)
			return
		}
		httpError(w, http.StatusNotFound, "not found: "+filePath)
		return
	}

	httpError(w, http.StatusNotFound, "not found: "+filePath)
}

func (s *Server) serveMarkdownPage(w http.ResponseWriter, src []byte, repoBase, repoName, filePath, ref string, navResult nav.ParseResult, trusted bool) {
	result, err := render.Render(src, repoBase, filePath, ref, trusted)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "render error: "+err.Error())
		return
	}
	title := headingTitle(string(src))
	if title == "" {
		title = filePath
	}
	navItems := sectionsToNav(navResult.Sections)
	activeIdx := -1
	var sections []nav.Section
	if navResult.IsMulti && len(navResult.Sections) > 0 {
		sections = navResult.Sections
		_, idx, found := nav.FindActiveSection(navResult.Sections, filePath)
		if found {
			activeIdx = idx
		}
	}
	data := docData{
		Title:         title,
		Content:       result.Content,
		TOC:           result.TOC,
		Breadcrumbs:   buildBreadcrumbs(repoBase, filePath, ref),
		RepoBase:      repoBase,
		RepoName:      repoName,
		Ref:           ref,
		Nav:           navItems,
		CurrentPath:   filePath,
		Sections:      sections,
		ActiveSection: activeIdx,
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

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title         string
		Repos         []string
		Locals        []string
		TOC           template.HTML
		RepoBase      string
		RepoName      string
		Breadcrumbs   []breadcrumb
		Nav           []nav.Item
		Sections      []nav.Section
		ActiveSection int
	}{
		Title:  "Folio",
		Repos:  s.store.RepoKeys(),
		Locals: s.store.LocalLabels(),
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

func firstNavLeaf(items []nav.Item) string {
	for _, item := range items {
		if item.Path != "" {
			return item.Path
		}
		if child := firstNavLeaf(item.Children); child != "" {
			return child
		}
	}
	return ""
}

func navCoversPath(items []nav.Item, filePath string) bool {
	for _, item := range items {
		if item.Path != "" {
			if item.Path == filePath {
				return true
			}
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

func loadNavResult(repo gitstore.Repository, hash plumbing.Hash) nav.ParseResult {
	if data, err := repo.ReadBlob(hash, "folio.yml"); err == nil {
		if result, err := nav.ParseWithSections(data); err == nil {
			return result
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
	return nav.ParseResult{Sections: []nav.Section{{Nav: nav.AutoGenerate(walker, "")}}}
}

func headingTitle(src string) string {
	for _, line := range strings.SplitN(src, "\n", 100) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
