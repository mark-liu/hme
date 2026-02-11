package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{
		BaseURL:    srv.URL,
		Cookies:    "X-APPLE-WEBAUTH-USER=d12345%200; X-APPLE-WEBAUTH-TOKEN=tok",
		DSID:       "d12345",
		ClientID:   "test-client-id",
		HTTPClient: srv.Client(),
	}
}

func TestGenerate_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/hme/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify required query params
		q := r.URL.Query()
		if q.Get("dsid") != "d12345" {
			t.Errorf("dsid = %q, want d12345", q.Get("dsid"))
		}
		// Verify required headers
		if r.Header.Get("Origin") != "https://www.icloud.com" {
			t.Errorf("missing Origin header")
		}
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: true,
			Result:  GenerateResult{Hme: "random123@icloud.com"},
		})
	})

	email, err := c.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if email != "random123@icloud.com" {
		t.Errorf("email = %q, want random123@icloud.com", email)
	}
}

func TestGenerate_APIError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   &APIError{ErrorCode: 1234, ErrorMessage: "quota exceeded"},
		})
	})

	_, err := c.Generate()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "quota exceeded" {
		t.Errorf("error = %q, want 'quota exceeded'", err)
	}
}

func TestGenerate_401(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := c.Generate()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "cookies expired. Run 'hme auth' to refresh" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerate_421(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(421)
	})

	_, err := c.Generate()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "cookies expired. Run 'hme auth' to refresh" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerate_429(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := c.Generate()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "rate limited. Wait a few minutes" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerate_MalformedJSON(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})

	_, err := c.Generate()
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestGenerate_Timeout(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	})
	c.HTTPClient.Timeout = 100 * time.Millisecond

	_, err := c.Generate()
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestReserve_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/hme/reserve" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req ReserveRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Hme != "test@icloud.com" {
			t.Errorf("hme = %q", req.Hme)
		}
		if req.Label != "Test Label" {
			t.Errorf("label = %q", req.Label)
		}
		json.NewEncoder(w).Encode(ReserveResponse{Success: true})
	})

	err := c.Reserve("test@icloud.com", "Test Label", "a note")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
}

func TestReserve_APIError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ReserveResponse{
			Success: false,
			Error:   &APIError{ErrorCode: 5, Reason: "duplicate label"},
		})
	})

	err := c.Reserve("test@icloud.com", "Dup", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "duplicate label" {
		t.Errorf("error = %q", err)
	}
}

func TestList_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v2/hme/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(ListResponse{
			Success: true,
			Result: ListResult{
				HmeEmails: []HmeEmail{
					{Hme: "a@icloud.com", Label: "GitHub", IsActive: true},
					{Hme: "b@icloud.com", Label: "Spam", IsActive: false},
				},
			},
		})
	})

	emails, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("len = %d, want 2", len(emails))
	}
	if emails[0].Hme != "a@icloud.com" {
		t.Errorf("first email = %q", emails[0].Hme)
	}
}

func TestList_Empty(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ListResponse{
			Success: true,
			Result:  ListResult{HmeEmails: []HmeEmail{}},
		})
	})

	emails, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(emails) != 0 {
		t.Errorf("len = %d, want 0", len(emails))
	}
}

func TestNewClient_BadCookies(t *testing.T) {
	_, err := NewClient("no-dsid-here=foo; bar=baz")
	if err == nil {
		t.Fatal("expected error for cookies without dsid")
	}
}
