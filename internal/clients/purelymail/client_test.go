package purelymail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("test-api-token")
	c.httpClient = srv.Client()
	c.baseURL = srv.URL
	return c
}

// TestIsNotFoundPrefixMatching uses the exact error strings captured against
// the live PurelyMail API (see this provider's task brief / README) as
// fixtures, so a future PurelyMail wording change breaks this test suite
// loudly instead of silently breaking prod reconciliation.
func TestIsNotFoundPrefixMatching(t *testing.T) {
	cases := []struct {
		name               string
		err                error
		wantUserNotFound   bool
		wantDomainNotFound bool
		wantRuleNotFound   bool
		wantInvalidToken   bool
	}{
		{
			name:             "unknown user",
			err:              &APIError{Code: "internalError", Message: "Unknown user alice@example.com"},
			wantUserNotFound: true,
		},
		{
			name:               "unknown domain",
			err:                &APIError{Code: "internalError", Message: `Unknown domain "example.com"`},
			wantDomainNotFound: true,
		},
		{
			name:             "unknown routing rule",
			err:              &APIError{Code: "internalError", Message: "Unknown rule 42"},
			wantRuleNotFound: true,
		},
		{
			name:             "invalid token",
			err:              &APIError{Code: "invalidToken", Message: "Token not valid."},
			wantInvalidToken: true,
		},
		{
			name: "generic internalError does not match any not-found prefix",
			err:  &APIError{Code: "internalError", Message: "Something went wrong on our end"},
		},
		{
			// "unknown user" above already proves that a user-shaped message
			// satisfies IsUserNotFound; this proves a domain-shaped message does
			// NOT also satisfy it, i.e. the prefix match is exclusive per resource
			// type rather than "any Unknown ... message".
			name:               "unknown domain message does not satisfy IsUserNotFound",
			err:                &APIError{Code: "internalError", Message: `Unknown domain "example.com"`},
			wantDomainNotFound: true,
		},
		{
			name: "non-APIError never matches",
			err:  errNonAPI{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUserNotFound(tc.err); got != tc.wantUserNotFound {
				t.Errorf("IsUserNotFound(%v) = %v, want %v", tc.err, got, tc.wantUserNotFound)
			}
			if got := IsDomainNotFound(tc.err); got != tc.wantDomainNotFound {
				t.Errorf("IsDomainNotFound(%v) = %v, want %v", tc.err, got, tc.wantDomainNotFound)
			}
			if got := IsRoutingRuleNotFound(tc.err); got != tc.wantRuleNotFound {
				t.Errorf("IsRoutingRuleNotFound(%v) = %v, want %v", tc.err, got, tc.wantRuleNotFound)
			}
			if got := IsInvalidToken(tc.err); got != tc.wantInvalidToken {
				t.Errorf("IsInvalidToken(%v) = %v, want %v", tc.err, got, tc.wantInvalidToken)
			}
		})
	}
}

type errNonAPI struct{}

func (errNonAPI) Error() string { return "some other error" }

func TestDoDecodesSuccessEnvelope(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/getOwnershipCode" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get(apiTokenHeader); got != "test-api-token" {
			t.Fatalf("unexpected %s header: %q", apiTokenHeader, got)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":   "success",
			"result": map[string]any{"code": "abc123"},
		})
	})

	code, err := c.GetOwnershipCode(context.Background())
	if err != nil {
		t.Fatalf("GetOwnershipCode: %v", err)
	}
	if code != "abc123" {
		t.Fatalf("unexpected code: %q", code)
	}
}

func TestDoDecodesErrorEnvelopeRegardlessOfHTTP200(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// PurelyMail always answers 200, even for a logical error.
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":    "error",
			"code":    "internalError",
			"message": "Unknown user alice@example.com",
		})
	})

	_, err := c.GetUser(context.Background(), "alice@example.com")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsUserNotFound(err) {
		t.Fatalf("expected IsUserNotFound(err) to be true, got err: %v", err)
	}
}

func TestDoErrorsOnUnexpectedHTTPStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	})

	_, err := c.GetOwnershipCode(context.Background())
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestListDomains(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/listDomains" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req listDomainsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.IncludeShared {
			t.Fatalf("expected includeShared=false, got true")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"domains": []map[string]any{
					{
						"name":                  "example.com",
						"allowAccountReset":     true,
						"symbolicSubaddressing": false,
						"isShared":              false,
						"dnsSummary": map[string]any{
							"passesMx": true, "passesSpf": true, "passesDkim": false, "passesDmarc": false,
						},
					},
				},
			},
		})
	})

	domains, err := c.ListDomains(context.Background(), false)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(domains) != 1 || domains[0].Name != "example.com" {
		t.Fatalf("unexpected domains: %+v", domains)
	}
	if !domains[0].DNSSummary.PassesMx {
		t.Fatalf("expected dnsSummary.passesMx to be true")
	}
}

func TestCreateRoutingRuleDoesNotReturnID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/createRoutingRule" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req createRoutingRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.DomainName != "example.com" {
			t.Fatalf("unexpected domainName: %q", req.DomainName)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "result": map[string]any{}})
	})

	err := c.CreateRoutingRule(context.Background(), CreateRoutingRuleInput{
		DomainName:      "example.com",
		MatchUser:       "sales",
		TargetAddresses: []string{"alice@example.com"},
	})
	if err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}
}

func TestListRoutingRulesReturnsID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"rules": []map[string]any{
					{
						"id":              float64(42),
						"domainName":      "example.com",
						"prefix":          false,
						"matchUser":       "sales",
						"targetAddresses": []string{"alice@example.com"},
						"catchall":        false,
					},
				},
			},
		})
	})

	rules, err := c.ListRoutingRules(context.Background())
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != 42 {
		t.Fatalf("unexpected rules: %+v", rules)
	}
}

func TestParseAndFormatRoutingRuleID(t *testing.T) {
	id, err := ParseRoutingRuleID(FormatRoutingRuleID(42))
	if err != nil {
		t.Fatalf("ParseRoutingRuleID: %v", err)
	}
	if id != 42 {
		t.Fatalf("unexpected id: %d", id)
	}
}
