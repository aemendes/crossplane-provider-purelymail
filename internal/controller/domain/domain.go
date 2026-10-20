// Package domain manages the lifecycle of PurelymailDomain managed resources.
package domain

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	crossplanecontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	v1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/domain/v1alpha1"
	apisv1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/v1alpha1"
	"github.com/aemendes/crossplane-provider-purelymail/internal/clients/purelymail"
)

const (
	errNotDomain    = "managed resource is not a PurelymailDomain custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errObserve = "cannot observe PurelyMail domain"
	errCreate  = "cannot create PurelyMail domain"
	errUpdate  = "cannot update PurelyMail domain"
	errDelete  = "cannot delete PurelyMail domain"
)

// Setup adds a controller that reconciles PurelymailDomain managed resources.
func Setup(mgr ctrl.Manager, o crossplanecontroller.Options) error {
	name := managed.ControllerName(v1alpha1.PurelymailDomainKind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.PurelymailDomainKind)),
		managed.WithExternalConnecter(&connector{
			kube:        mgr.GetClient(),
			usage:       resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newClientFn: purelymail.NewClient,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
		managed.WithManagementPolicies(),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(controller.Options{MaxConcurrentReconciles: o.MaxConcurrentReconciles}).
		For(&v1alpha1.PurelymailDomain{}).
		Complete(r)
}

// connector produces an ExternalClient by reading the PurelyMail API token out
// of the ProviderConfig referenced by the PurelymailDomain being reconciled.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	newClientFn func(apiToken string) *purelymail.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.PurelymailDomain)
	if !ok {
		return nil, errors.New(errNotDomain)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	data, err := resource.CommonCredentialExtractor(ctx, pc.Spec.Credentials.Source, c.kube, pc.Spec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	return &external{client: c.newClientFn(string(data))}, nil
}

// external observes, creates, updates and deletes PurelyMail domains on behalf
// of a PurelymailDomain managed resource.
type external struct {
	client *purelymail.Client
}

// Compile-time checks that connector/external satisfy the crossplane-runtime
// interfaces managed.NewReconciler requires. A missing method here surfaces
// only as a runtime panic on provider startup.
var (
	_ managed.ExternalConnecter = &connector{}
	_ managed.ExternalClient    = &external{}
)

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.PurelymailDomain)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotDomain)
	}

	// There is no getDomain operation in PurelyMail's API, so an existing
	// domain is always found by listing every domain on the account and
	// filtering client-side by name, whether or not external-name is set yet.
	found, err := e.findByName(ctx, cr.Spec.ForProvider.DomainName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}
	if found == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	lateInit := false
	if meta.GetExternalName(cr) == "" {
		meta.SetExternalName(cr, found.Name)
		lateInit = true
	}

	cr.SetConditions(xpv1.Available())
	cr.Status.AtProvider = v1alpha1.DomainObservation{
		IsShared: found.IsShared,
		DNSSummary: v1alpha1.DNSSummary{
			PassesMx:    found.DNSSummary.PassesMx,
			PassesSpf:   found.DNSSummary.PassesSpf,
			PassesDkim:  found.DNSSummary.PassesDkim,
			PassesDmarc: found.DNSSummary.PassesDmarc,
		},
	}

	upToDate := true
	if cr.Spec.ForProvider.AllowAccountReset != nil && *cr.Spec.ForProvider.AllowAccountReset != found.AllowAccountReset {
		upToDate = false
	}
	if cr.Spec.ForProvider.SymbolicSubaddressing != nil && *cr.Spec.ForProvider.SymbolicSubaddressing != found.SymbolicSubaddressing {
		upToDate = false
	}

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        upToDate,
		ResourceLateInitialized: lateInit,
	}, nil
}

func (e *external) findByName(ctx context.Context, name string) (*purelymail.ApiDomainInfo, error) {
	domains, err := e.client.ListDomains(ctx, false)
	if err != nil {
		return nil, err
	}
	for i := range domains {
		if domains[i].Name == name {
			return &domains[i], nil
		}
	}
	return nil, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.PurelymailDomain)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotDomain)
	}

	cr.SetConditions(xpv1.Creating())

	if err := e.client.AddDomain(ctx, cr.Spec.ForProvider.DomainName); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	// addDomain itself only accepts the domain name — allowAccountReset and
	// symbolicSubaddressing must be pushed with a follow-up updateDomainSettings
	// call, or the resource would otherwise show up as immediately out of date
	// on the very next Observe.
	if cr.Spec.ForProvider.AllowAccountReset != nil || cr.Spec.ForProvider.SymbolicSubaddressing != nil {
		in := purelymail.UpdateDomainSettingsInput{
			Name:                  cr.Spec.ForProvider.DomainName,
			AllowAccountReset:     cr.Spec.ForProvider.AllowAccountReset,
			SymbolicSubaddressing: cr.Spec.ForProvider.SymbolicSubaddressing,
		}
		if err := e.client.UpdateDomainSettings(ctx, in); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
		}
	}

	meta.SetExternalName(cr, cr.Spec.ForProvider.DomainName)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.PurelymailDomain)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotDomain)
	}

	in := purelymail.UpdateDomainSettingsInput{
		Name:                  meta.GetExternalName(cr),
		AllowAccountReset:     cr.Spec.ForProvider.AllowAccountReset,
		SymbolicSubaddressing: cr.Spec.ForProvider.SymbolicSubaddressing,
	}
	if err := e.client.UpdateDomainSettings(ctx, in); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.PurelymailDomain)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotDomain)
	}

	cr.SetConditions(xpv1.Deleting())

	if err := e.client.DeleteDomain(ctx, meta.GetExternalName(cr)); err != nil {
		if purelymail.IsDomainNotFound(err) {
			return managed.ExternalDelete{}, nil
		}
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}
