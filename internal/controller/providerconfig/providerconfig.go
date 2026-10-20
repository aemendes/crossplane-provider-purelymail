// Package providerconfig reconciles ProviderConfigs by tracking their usage.
package providerconfig

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	xpcontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/aemendes/crossplane-provider-purelymail/apis/v1alpha1"
)

// Setup adds a controller that reconciles ProviderConfigs by accounting for
// their current usage.
func Setup(mgr ctrl.Manager, o xpcontroller.Options) error {
	name := providerconfig.ControllerName(v1alpha1.ProviderConfigGroupKind)

	of := resource.ProviderConfigKinds{
		Config:    v1alpha1.SchemeGroupVersion.WithKind("ProviderConfig"),
		Usage:     v1alpha1.SchemeGroupVersion.WithKind("ProviderConfigUsage"),
		UsageList: v1alpha1.SchemeGroupVersion.WithKind("ProviderConfigUsageList"),
	}

	r := providerconfig.NewReconciler(mgr, of,
		providerconfig.WithLogger(o.Logger.WithValues("controller", name)),
		providerconfig.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(controller.Options{MaxConcurrentReconciles: o.MaxConcurrentReconciles}).
		For(&v1alpha1.ProviderConfig{}).
		Watches(&v1alpha1.ProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
		Complete(r)
}
