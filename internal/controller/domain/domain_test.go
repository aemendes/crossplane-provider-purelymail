package domain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/meta"

	v1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/domain/v1alpha1"
	"github.com/aemendes/crossplane-provider-purelymail/internal/clients/purelymail"
)

func newDomain(name string) *v1alpha1.PurelymailDomain {
	d := &v1alpha1.PurelymailDomain{}
	d.SetName(name)
	d.Spec.ForProvider.DomainName = name
	return d
}

func TestObserveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":   "success",
			"result": map[string]any{"domains": []any{}},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newDomain("example.com")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false when the domain isn't in listDomains")
	}
}

func TestObserveFoundAdoptsExternalName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"domains": []map[string]any{
					{
						"name": "example.com", "allowAccountReset": true, "symbolicSubaddressing": false,
						"isShared":   false,
						"dnsSummary": map[string]any{"passesMx": true, "passesSpf": true, "passesDkim": true, "passesDmarc": true},
					},
				},
			},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newDomain("example.com")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists {
		t.Fatal("expected ResourceExists=true")
	}
	if !obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=true when spec leaves optional fields unset")
	}
	if !obs.ResourceLateInitialized {
		t.Fatal("expected ResourceLateInitialized=true so the discovered external-name is persisted")
	}
	if meta.GetExternalName(cr) != "example.com" {
		t.Fatalf("expected external-name to be adopted as example.com, got %q", meta.GetExternalName(cr))
	}
	if !cr.Status.AtProvider.DNSSummary.PassesMx {
		t.Fatal("expected status.atProvider.dnsSummary.passesMx to be populated")
	}
}

func TestObserveDetectsDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"domains": []map[string]any{
					{"name": "example.com", "allowAccountReset": false, "symbolicSubaddressing": false, "isShared": false, "dnsSummary": map[string]any{}},
				},
			},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newDomain("example.com")
	wantTrue := true
	cr.Spec.ForProvider.AllowAccountReset = &wantTrue

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=false when spec.allowAccountReset differs from PurelyMail's value")
	}
}

func TestCreateSetsExternalName(t *testing.T) {
	var addCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/addDomain" {
			addCalled = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "success", "result": map[string]any{}})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newDomain("example.com")

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !addCalled {
		t.Fatal("expected addDomain to be called")
	}
	if meta.GetExternalName(cr) != "example.com" {
		t.Fatalf("expected external-name to be set to example.com, got %q", meta.GetExternalName(cr))
	}
}

func TestDeleteIsNoopWhenAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "error", "code": "internalError", "message": `Unknown domain "example.com"`,
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newDomain("example.com")
	meta.SetExternalName(cr, "example.com")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("expected no error deleting an already-gone domain, got: %v", err)
	}
}

func TestDeletePropagatesUnrelatedInternalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "error", "code": "internalError", "message": "Something went wrong on our end",
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newDomain("example.com")
	meta.SetExternalName(cr, "example.com")

	if _, err := e.Delete(context.Background(), cr); err == nil {
		t.Fatal("expected an unrelated internalError to propagate rather than being treated as not-found")
	}
}
