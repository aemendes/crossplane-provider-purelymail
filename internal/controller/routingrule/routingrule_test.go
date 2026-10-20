package routingrule

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/meta"

	v1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/routingrule/v1alpha1"
	"github.com/aemendes/crossplane-provider-purelymail/internal/clients/purelymail"
)

func newRule(domain, matchUser string, targets ...string) *v1alpha1.PurelymailRoutingRule {
	r := &v1alpha1.PurelymailRoutingRule{}
	r.SetName("sales-routing")
	r.Spec.ForProvider.DomainName = domain
	r.Spec.ForProvider.MatchUser = matchUser
	r.Spec.ForProvider.TargetAddresses = targets
	return r
}

func TestObserveNoExternalNameAndNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "result": map[string]any{"rules": []any{}}})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "alice@example.com")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false when no rule matches by tuple")
	}
}

func TestObserveAdoptsByTuple(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"rules": []map[string]any{
					{
						"id": float64(7), "domainName": "example.com", "prefix": false,
						"matchUser": "sales", "targetAddresses": []string{"alice@example.com"}, "catchall": false,
					},
				},
			},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "alice@example.com")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists || !obs.ResourceUpToDate || !obs.ResourceLateInitialized {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if meta.GetExternalName(cr) != "7" {
		t.Fatalf("expected external-name to be adopted as \"7\", got %q", meta.GetExternalName(cr))
	}
}

func TestObserveAmbiguousMatchesIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"rules": []map[string]any{
					{"id": float64(7), "domainName": "example.com", "prefix": false, "matchUser": "sales", "targetAddresses": []string{"alice@example.com"}, "catchall": false},
					{"id": float64(8), "domainName": "example.com", "prefix": false, "matchUser": "sales", "targetAddresses": []string{"alice@example.com"}, "catchall": false},
				},
			},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "alice@example.com")

	if _, err := e.Observe(context.Background(), cr); err == nil {
		t.Fatal("expected an error when more than one routing rule matches the exact same tuple")
	}
}

func TestObserveByIDDetectsDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"rules": []map[string]any{
					{"id": float64(7), "domainName": "example.com", "prefix": false, "matchUser": "sales", "targetAddresses": []string{"alice@example.com"}, "catchall": false},
				},
			},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "bob@example.com")
	meta.SetExternalName(cr, "7")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists {
		t.Fatal("expected ResourceExists=true")
	}
	if obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=false when targetAddresses differ from spec")
	}
}

func TestObserveByIDGoneReturnsNotExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "result": map[string]any{"rules": []any{}}})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "alice@example.com")
	meta.SetExternalName(cr, "7")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false once the rule id is no longer in listRoutingRules")
	}
}

func TestCreateRecoversIDViaListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/createRoutingRule":
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "result": map[string]any{}})
		case "/api/v0/listRoutingRules":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type": "success",
				"result": map[string]any{
					"rules": []map[string]any{
						{"id": float64(99), "domainName": "example.com", "prefix": false, "matchUser": "sales", "targetAddresses": []string{"alice@example.com"}, "catchall": false},
					},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "alice@example.com")

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if meta.GetExternalName(cr) != "99" {
		t.Fatalf("expected external-name to be set to the recovered id 99, got %q", meta.GetExternalName(cr))
	}
	if cr.Status.AtProvider.ID != 0 {
		// Create doesn't populate status itself, only external-name; the next
		// Observe call fills in status.atProvider.id.
		t.Fatalf("did not expect Create to populate status.atProvider.id, got %d", cr.Status.AtProvider.ID)
	}
}

func TestCreateErrorsWhenIDCannotBeRecovered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/createRoutingRule":
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "result": map[string]any{}})
		case "/api/v0/listRoutingRules":
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "result": map[string]any{"rules": []any{}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "alice@example.com")

	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("expected Create to fail loudly when the new rule can't be found afterward, rather than leaving external-name unset")
	}
}

func TestDeleteIsNoopWhenAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "code": "internalError", "message": "Unknown rule 7"})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newRule("example.com", "sales", "alice@example.com")
	meta.SetExternalName(cr, "7")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("expected no error deleting an already-gone rule, got: %v", err)
	}
}
