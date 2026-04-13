package dashboard_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLoginPageGet(t *testing.T) {
	ts, _ := newTestDashboard(t)

	resp, err := http.Get(ts.URL + "/-/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "<form") {
		t.Error("expected response body to contain <form")
	}
	if !strings.Contains(bodyStr, "/-/api/v1/auth/login") {
		t.Error("expected response body to contain /-/api/v1/auth/login")
	}
}
