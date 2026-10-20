// Package apis contains Kubernetes API for the PurelyMail provider.
package apis

import (
	"k8s.io/apimachinery/pkg/runtime"

	domainv1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/domain/v1alpha1"
	routingrulev1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/routingrule/v1alpha1"
	userv1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/user/v1alpha1"
	purelymailv1alpha1 "github.com/aemendes/crossplane-provider-purelymail/apis/v1alpha1"
)

// AddToSchemes may be used to add all resources defined in the project to a Scheme.
var AddToSchemes runtime.SchemeBuilder = runtime.SchemeBuilder{
	purelymailv1alpha1.SchemeBuilder.AddToScheme,
	domainv1alpha1.SchemeBuilder.AddToScheme,
	userv1alpha1.SchemeBuilder.AddToScheme,
	routingrulev1alpha1.SchemeBuilder.AddToScheme,
}

// AddToScheme adds all Resources to the Scheme.
func AddToScheme(s *runtime.Scheme) error {
	return AddToSchemes.AddToScheme(s)
}
