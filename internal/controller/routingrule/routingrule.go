// Package routingrule manages the lifecycle of PurelymailRoutingRule managed resources.
package routingrule

import (
	"context"
	"sort"

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

	v1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/routingrule/v1alpha1"
	apisv1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/v1alpha1"
	"github.com/aemendes/crossplane-provider-purelymail/internal/clients/purelymail"
)

const (
	errNotRule      = "managed resource is not a PurelymailRoutingRule custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errObserve         = "cannot observe PurelyMail routing rule"
	errCreate          = "cannot create PurelyMail routing rule"
	errUpdate          = "cannot update PurelyMail routing rule"
	errDelete          = "cannot delete PurelyMail routing rule"
	errParseID         = "cannot parse external-name as a routing rule id"
	errAmbiguous       = "more than one PurelyMail routing rule matches this resource's exact fields; refusing to guess which one is ours"
	errFindAfterCreate = "created the routing rule but could not recover its id afterward"
)

// Setup adds a controller that reconciles PurelymailRoutingRule managed resources.
func Setup(mgr ctrl.Manager, o crossplanecontroller.Options) error {
	name := managed.ControllerName(v1alpha1.PurelymailRoutingRuleKind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.PurelymailRoutingRuleKind)),
		managed.WithExternalConnecter(&connector{
			kube:        mgr.GetClient(),
			usage:       resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newClientFn: purelymail.NewClient,
		}),
		// Disable the reconciler's default initializer for the same reason
		// provider-neon's Project does: it would set external-name to this
		// resource's own metadata.name before the first Observe ever runs,
		// making the id-recovery/adopt-by-tuple branch below (which only fires
		// when external-name is empty) unreachable — defeating its purpose of
		// avoiding an orphaned, undiscoverable PurelyMail routing rule if a
		// prior Create's response was lost before its id could be persisted.
		managed.WithInitializers(),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
		managed.WithManagementPolicies(),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(controller.Options{MaxConcurrentReconciles: o.MaxConcurrentReconciles}).
		For(&v1alpha1.PurelymailRoutingRule{}).
		Complete(r)
}

// connector produces an ExternalClient by reading the PurelyMail API token out
// of the ProviderConfig referenced by the PurelymailRoutingRule being reconciled.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	newClientFn func(apiToken string) *purelymail.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.PurelymailRoutingRule)
	if !ok {
		return nil, errors.New(errNotRule)
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

// external observes, creates, updates and deletes PurelyMail routing rules on
// behalf of a PurelymailRoutingRule managed resource.
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

func desired(cr *v1alpha1.PurelymailRoutingRule) purelymail.CreateRoutingRuleInput {
	return purelymail.CreateRoutingRuleInput{
		DomainName:      cr.Spec.ForProvider.DomainName,
		Prefix:          cr.Spec.ForProvider.Prefix,
		MatchUser:       cr.Spec.ForProvider.MatchUser,
		TargetAddresses: cr.Spec.ForProvider.TargetAddresses,
		Catchall:        cr.Spec.ForProvider.Catchall,
	}
}

// matches reports whether rule has exactly the fields in in. TargetAddresses
// is compared as a set, not an ordered list — PurelyMail gives no ordering
// guarantee on what listRoutingRules echoes back relative to what was POSTed.
func matches(rule purelymail.RoutingRule, in purelymail.CreateRoutingRuleInput) bool {
	if rule.DomainName != in.DomainName || rule.Prefix != in.Prefix || rule.MatchUser != in.MatchUser || rule.Catchall != in.Catchall {
		return false
	}
	return sameAddressSet(rule.TargetAddresses, in.TargetAddresses)
}

func sameAddressSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa, sb := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(sa)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

// findByTuple lists every routing rule on the account and returns the one
// exactly matching in's fields. PurelyMail's createRoutingRule response never
// includes the new rule's id, and there is no per-rule get, so this is the
// only way to learn (or re-learn) it. Returns an error if more than one rule
// matches — the API permits duplicate rules, and silently picking one would
// risk operating on the wrong one, so this fails loudly instead per this
// repo's convention (see codeInternalError handling in the client).
func (e *external) findByTuple(ctx context.Context, in purelymail.CreateRoutingRuleInput) (*purelymail.RoutingRule, error) {
	rules, err := e.client.ListRoutingRules(ctx)
	if err != nil {
		return nil, err
	}
	var found *purelymail.RoutingRule
	for i := range rules {
		if matches(rules[i], in) {
			if found != nil {
				return nil, errors.New(errAmbiguous)
			}
			found = &rules[i]
		}
	}
	return found, nil
}

func (e *external) findByID(ctx context.Context, id int64) (*purelymail.RoutingRule, error) {
	rules, err := e.client.ListRoutingRules(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i], nil
		}
	}
	return nil, nil
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.PurelymailRoutingRule)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotRule)
	}

	in := desired(cr)

	extName := meta.GetExternalName(cr)
	if extName == "" {
		found, err := e.findByTuple(ctx, in)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
		}
		if found == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		meta.SetExternalName(cr, purelymail.FormatRoutingRuleID(found.ID))
		cr.SetConditions(xpv1.Available())
		cr.Status.AtProvider = v1alpha1.RoutingRuleObservation{ID: found.ID}
		return managed.ExternalObservation{
			ResourceExists:          true,
			ResourceUpToDate:        true,
			ResourceLateInitialized: true,
		}, nil
	}

	id, err := purelymail.ParseRoutingRuleID(extName)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errParseID)
	}

	found, err := e.findByID(ctx, id)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}
	if found == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cr.SetConditions(xpv1.Available())
	cr.Status.AtProvider = v1alpha1.RoutingRuleObservation{ID: found.ID}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: matches(*found, in),
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.PurelymailRoutingRule)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotRule)
	}

	cr.SetConditions(xpv1.Creating())

	in := desired(cr)
	if err := e.client.CreateRoutingRule(ctx, in); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	found, err := e.findByTuple(ctx, in)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errFindAfterCreate)
	}
	if found == nil {
		return managed.ExternalCreation{}, errors.New(errFindAfterCreate)
	}

	meta.SetExternalName(cr, purelymail.FormatRoutingRuleID(found.ID))

	return managed.ExternalCreation{}, nil
}

// Update implements the only option PurelyMail's API leaves for changing a
// routing rule: delete the old one and create its replacement, then move the
// external-name annotation to the new id. There is no update/patch operation
// for routing rules at all.
func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.PurelymailRoutingRule)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotRule)
	}

	oldID, err := purelymail.ParseRoutingRuleID(meta.GetExternalName(cr))
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errParseID)
	}

	if err := e.client.DeleteRoutingRule(ctx, oldID); err != nil && !purelymail.IsRoutingRuleNotFound(err) {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	in := desired(cr)
	if err := e.client.CreateRoutingRule(ctx, in); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	found, err := e.findByTuple(ctx, in)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errFindAfterCreate)
	}
	if found == nil {
		return managed.ExternalUpdate{}, errors.New(errFindAfterCreate)
	}

	meta.SetExternalName(cr, purelymail.FormatRoutingRuleID(found.ID))

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.PurelymailRoutingRule)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotRule)
	}

	cr.SetConditions(xpv1.Deleting())

	id, err := purelymail.ParseRoutingRuleID(meta.GetExternalName(cr))
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errParseID)
	}

	if err := e.client.DeleteRoutingRule(ctx, id); err != nil {
		if purelymail.IsRoutingRuleNotFound(err) {
			return managed.ExternalDelete{}, nil
		}
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}
