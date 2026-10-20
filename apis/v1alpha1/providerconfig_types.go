package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A ProviderConfigSpec defines the desired state of a ProviderConfig.
type ProviderConfigSpec struct {
	// Credentials required to authenticate to the PurelyMail API.
	Credentials ProviderCredentials `json:"credentials"`
}

// ProviderCredentials required to authenticate to the PurelyMail API.
type ProviderCredentials struct {
	// Source of the credentials.
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source xpv1.CredentialsSource `json:"source"`

	xpv1.CommonCredentialSelectors `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// A ProviderConfig configures a PurelyMail provider.
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec        `json:"spec"`
	Status xpv1.ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig.
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// GetCondition of this ProviderConfig.
func (p *ProviderConfig) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return p.Status.GetCondition(ct)
}

// SetConditions of this ProviderConfig.
func (p *ProviderConfig) SetConditions(c ...xpv1.Condition) {
	p.Status.SetConditions(c...)
}

// GetUsers of this ProviderConfig.
func (p *ProviderConfig) GetUsers() int64 {
	return p.Status.Users
}

// SetUsers of this ProviderConfig.
func (p *ProviderConfig) SetUsers(i int64) {
	p.Status.Users = i
}
