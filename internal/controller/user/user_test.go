package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/crossplane-runtime/pkg/meta"

	v1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/user/v1alpha1"
	"github.com/aemendes/crossplane-provider-purelymail/internal/clients/purelymail"
)

func newUser(username, domain string) *v1alpha1.PurelymailUser {
	u := &v1alpha1.PurelymailUser{}
	u.SetName(username)
	u.Spec.ForProvider.Username = username
	u.Spec.ForProvider.DomainName = domain
	return u
}

func TestFullAddress(t *testing.T) {
	cr := newUser("alice", "example.com")
	if got := fullAddress(cr); got != "alice@example.com" {
		t.Fatalf("unexpected full address: %q", got)
	}
}

func TestObserveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "error", "code": "internalError", "message": "Unknown user alice@example.com",
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newUser("alice", "example.com")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false for an unknown user")
	}
}

func TestObserveLateInitializesObservableFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"enableSearchIndexing":           true,
				"recoveryEnabled":                false,
				"requireTwoFactorAuthentication": false,
				"enableSpamFiltering":            true,
				"resetMethods":                   []any{},
			},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newUser("alice", "example.com")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if !obs.ResourceLateInitialized {
		t.Fatal("expected ResourceLateInitialized=true when spec fields were nil and got adopted from getUser")
	}
	if cr.Spec.ForProvider.EnableSearchIndexing == nil || !*cr.Spec.ForProvider.EnableSearchIndexing {
		t.Fatal("expected spec.enableSearchIndexing to be late-initialized to true")
	}
	if meta.GetExternalName(cr) != "alice@example.com" {
		t.Fatalf("expected external-name to be set, got %q", meta.GetExternalName(cr))
	}
}

func TestObserveDetectsDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "success",
			"result": map[string]any{
				"enableSearchIndexing":           false,
				"requireTwoFactorAuthentication": false,
			},
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newUser("alice", "example.com")
	wantTrue := true
	cr.Spec.ForProvider.EnableSearchIndexing = &wantTrue

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=false when spec.enableSearchIndexing differs from getUser's value")
	}
}

func TestDeleteIsNoopWhenAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "error", "code": "internalError", "message": "Unknown user alice@example.com",
		})
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL)}
	cr := newUser("alice", "example.com")
	meta.SetExternalName(cr, "alice@example.com")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("expected no error deleting an already-gone user, got: %v", err)
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
	cr := newUser("alice", "example.com")
	meta.SetExternalName(cr, "alice@example.com")

	if _, err := e.Delete(context.Background(), cr); err == nil {
		t.Fatal("expected an unrelated internalError to propagate rather than being treated as not-found")
	}
}

func TestCreateFailsFastWithoutPasswordSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("did not expect any PurelyMail API call when the password secret cannot be resolved")
	}))
	defer srv.Close()

	e := &external{client: purelymail.NewClientWithBaseURL("token", srv.URL), kube: fake.NewClientBuilder().Build()}
	cr := newUser("alice", "example.com")
	cr.Spec.ForProvider.PasswordSecretRef.Name = "alice-password"
	cr.Spec.ForProvider.PasswordSecretRef.Namespace = "crossplane-system"
	cr.Spec.ForProvider.PasswordSecretRef.Key = "password"

	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("expected Create to fail when the password Secret does not exist")
	}
}
