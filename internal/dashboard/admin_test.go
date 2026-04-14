package dashboard_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAdminUsersPage_AsAdmin(t *testing.T) {
	srv, adminTok, _, _ := adminTestServerWithStore(t)

	req, _ := http.NewRequest("GET", srv.URL+"/-/dashboard/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: adminTok})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := new(strings.Builder)
	io.Copy(body, resp.Body)
	if !strings.Contains(body.String(), "admin@example.com") {
		t.Error("expected admin email in response body")
	}
	if !strings.Contains(body.String(), "user@example.com") {
		t.Error("expected regular user email in response body")
	}
}

func TestAdminUsersPage_NonAdmin(t *testing.T) {
	srv, _, regularTok, _ := adminTestServerWithStore(t)

	req, _ := http.NewRequest("GET", srv.URL+"/-/dashboard/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: regularTok})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
