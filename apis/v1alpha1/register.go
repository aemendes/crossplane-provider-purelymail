package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// Compile-time checks that these types satisfy the crossplane-runtime interfaces
// the reconcilers require. Without these, a missing method surfaces only as a
// runtime panic on provider startup.
var (
	_ resource.ProviderConfig          = &ProviderConfig{}
	_ resource.ProviderConfigUsage     = &ProviderConfigUsage{}
	_ resource.ProviderConfigUsageList = &ProviderConfigUsageList{}
)

// Package type metadata.
const (
	Group   = "purelymail.crossplane.io"
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register these objects.
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// ProviderConfigKind takes an unqualified kind and returns back a Group qualified GroupKind.
func ProviderConfigKind(kind string) schema.GroupKind {
	return schema.GroupKind{Group: Group, Kind: kind}
}

// ProviderConfigGroupKind is the root Kind for the ProviderConfig type, used to derive a controller name.
const ProviderConfigGroupKind = "ProviderConfig." + Group

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
	SchemeBuilder.Register(&ProviderConfigUsage{}, &ProviderConfigUsageList{})
}
