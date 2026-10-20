// Package user manages the lifecycle of PurelymailUser managed resources.
package user

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
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

	v1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/user/v1alpha1"
	apisv1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/v1alpha1"
	"github.com/aemendes/crossplane-provider-purelymail/internal/clients/purelymail"
)

const (
	errNotUser      = "managed resource is not a PurelymailUser custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errObserve     = "cannot observe PurelyMail user"
	errCreate      = "cannot create PurelyMail user"
	errUpdate      = "cannot update PurelyMail user"
	errDelete      = "cannot delete PurelyMail user"
	errGetPassword = "cannot get password secret"
	errNoPassword  = "passwordSecretRef did not resolve to a non-empty password"
)

// Setup adds a controller that reconciles PurelymailUser managed resources.
func Setup(mgr ctrl.Manager, o crossplanecontroller.Options) error {
	name := managed.ControllerName(v1alpha1.PurelymailUserKind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.PurelymailUserKind)),
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
		For(&v1alpha1.PurelymailUser{}).
		Complete(r)
}

// connector produces an ExternalClient by reading the PurelyMail API token out
// of the ProviderConfig referenced by the PurelymailUser being reconciled.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	newClientFn func(apiToken string) *purelymail.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.PurelymailUser)
	if !ok {
		return nil, errors.New(errNotUser)
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

	return &external{kube: c.kube, client: c.newClientFn(string(data))}, nil
}

// external observes, creates, updates and deletes PurelyMail users on behalf
// of a PurelymailUser managed resource.
type external struct {
	kube   client.Client
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

// fullAddress is the "local@domain" form PurelyMail expects for every
// operation except createUser (which splits it into userName+domainName) — see
// the GOTCHA comment on UserParameters.
func fullAddress(cr *v1alpha1.PurelymailUser) string {
	return cr.Spec.ForProvider.Username + "@" + cr.Spec.ForProvider.DomainName
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.PurelymailUser)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotUser)
	}

	addr := fullAddress(cr)
	info, err := e.client.GetUser(ctx, addr)
	if err != nil {
		if purelymail.IsUserNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	lateInit := false
	if meta.GetExternalName(cr) == "" {
		meta.SetExternalName(cr, addr)
		lateInit = true
	}

	cr.SetConditions(xpv1.Available())
	cr.Status.AtProvider = v1alpha1.UserObservation{
		RecoveryEnabled:     info.RecoveryEnabled,
		EnableSpamFiltering: info.EnableSpamFiltering,
		ResetMethods:        convertResetMethods(info.ResetMethods),
	}

	// EnableSearchIndexing and RequireTwoFactorAuthentication are the only two
	// spec fields PurelyMail's getUser echoes back, so they're the only two
	// this provider can either diff or late-initialize. Every other optional
	// field on UserParameters (password, recovery*, sendWelcomeEmail,
	// enablePasswordReset) is write-only in this API version and is therefore
	// never compared here — see the field-by-field comments on UserParameters.
	upToDate := true
	if cr.Spec.ForProvider.EnableSearchIndexing == nil {
		v := info.EnableSearchIndexing
		cr.Spec.ForProvider.EnableSearchIndexing = &v
		lateInit = true
	} else if *cr.Spec.ForProvider.EnableSearchIndexing != info.EnableSearchIndexing {
		upToDate = false
	}
	if cr.Spec.ForProvider.RequireTwoFactorAuthentication == nil {
		v := info.RequireTwoFactorAuthentication
		cr.Spec.ForProvider.RequireTwoFactorAuthentication = &v
		lateInit = true
	} else if *cr.Spec.ForProvider.RequireTwoFactorAuthentication != info.RequireTwoFactorAuthentication {
		upToDate = false
	}

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        upToDate,
		ResourceLateInitialized: lateInit,
	}, nil
}

func convertResetMethods(in []purelymail.ResetMethod) []v1alpha1.ResetMethod {
	if in == nil {
		return nil
	}
	out := make([]v1alpha1.ResetMethod, len(in))
	for i, m := range in {
		out[i] = v1alpha1.ResetMethod{
			Type:          m.Type,
			Target:        m.Target,
			Description:   m.Description,
			AllowMfaReset: m.AllowMfaReset,
		}
	}
	return out
}

func (e *external) getPassword(ctx context.Context, cr *v1alpha1.PurelymailUser) (string, error) {
	ref := cr.Spec.ForProvider.PasswordSecretRef
	s := &corev1.Secret{}
	if err := e.kube.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, s); err != nil {
		return "", errors.Wrap(err, errGetPassword)
	}
	pw, ok := s.Data[ref.Key]
	if !ok || len(pw) == 0 {
		return "", errors.New(errNoPassword)
	}
	return string(pw), nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.PurelymailUser)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotUser)
	}

	cr.SetConditions(xpv1.Creating())

	password, err := e.getPassword(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	in := purelymail.CreateUserInput{
		UserName:                 cr.Spec.ForProvider.Username,
		DomainName:               cr.Spec.ForProvider.DomainName,
		Password:                 password,
		EnablePasswordReset:      cr.Spec.ForProvider.EnablePasswordReset,
		RecoveryEmail:            cr.Spec.ForProvider.RecoveryEmail,
		RecoveryEmailDescription: cr.Spec.ForProvider.RecoveryEmailDescription,
		RecoveryPhone:            cr.Spec.ForProvider.RecoveryPhone,
		RecoveryPhoneDescription: cr.Spec.ForProvider.RecoveryPhoneDescription,
		EnableSearchIndexing:     cr.Spec.ForProvider.EnableSearchIndexing,
		SendWelcomeEmail:         cr.Spec.ForProvider.SendWelcomeEmail,
	}
	if err := e.client.CreateUser(ctx, in); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	addr := fullAddress(cr)

	// requireTwoFactorAuthentication has no createUser equivalent — it can only
	// be set via a follow-up modifyUser call.
	if cr.Spec.ForProvider.RequireTwoFactorAuthentication != nil {
		modIn := purelymail.ModifyUserInput{
			UserName:                       addr,
			RequireTwoFactorAuthentication: cr.Spec.ForProvider.RequireTwoFactorAuthentication,
		}
		if err := e.client.ModifyUser(ctx, modIn); err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
		}
	}

	meta.SetExternalName(cr, addr)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.PurelymailUser)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotUser)
	}

	// Password rotation is deliberately not wired up here: PurelyMail's getUser
	// never echoes a password back, so this provider has no way to tell whether
	// the Secret's current value differs from what PurelyMail has stored, and
	// blindly resending it on every unrelated Update would be a surprising side
	// effect. Rotate by deleting and recreating the resource instead (see the
	// PasswordSecretRef doc comment).
	in := purelymail.ModifyUserInput{
		UserName:                       meta.GetExternalName(cr),
		EnableSearchIndexing:           cr.Spec.ForProvider.EnableSearchIndexing,
		RequireTwoFactorAuthentication: cr.Spec.ForProvider.RequireTwoFactorAuthentication,
	}
	if err := e.client.ModifyUser(ctx, in); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.PurelymailUser)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotUser)
	}

	cr.SetConditions(xpv1.Deleting())

	if err := e.client.DeleteUser(ctx, meta.GetExternalName(cr)); err != nil {
		if purelymail.IsUserNotFound(err) {
			return managed.ExternalDelete{}, nil
		}
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}
