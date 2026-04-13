package dashboard_test

import (
	"net/http"
	"testing"
)

func TestDashboardRoutes_AllRequireAuth(t *testing.T) {
	ts, _, _ := newDashboardTestServer(t)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/-/dashboard/"},
		{http.MethodGet, "/-/dashboard/repos/new"},
		{http.MethodPost, "/-/dashboard/repos/new"},
		{http.MethodGet, "/-/dashboard/repos/1"},
		{http.MethodPost, "/-/dashboard/repos/1"},
		{http.MethodPost, "/-/dashboard/repos/1/delete"},
		{http.MethodPost, "/-/dashboard/repos/1/sync"},
		{http.MethodGet, "/-/dashboard/settings"},
		{http.MethodPost, "/-/dashboard/settings"},
		{http.MethodPost, "/-/dashboard/settings/unlink/github"},
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusFound {
				t.Errorf("want 302, got %d", resp.StatusCode)
			}
			loc := resp.Header.Get("Location")
			if loc != "/-/auth/login" {
				t.Errorf("want Location /-/auth/login, got %q", loc)
			}
		})
	}
}
