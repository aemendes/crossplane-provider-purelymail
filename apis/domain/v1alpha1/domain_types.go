package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DomainParameters define the desired state of a PurelyMail domain.
type DomainParameters struct {
	// DomainName is the domain to register with PurelyMail, e.g. "example.com".
	// Immutable after creation — PurelyMail has no rename operation for a domain,
	// and this provider uses the name itself as the external-name (there is no
	// separate domain id in PurelyMail's API). The domain's ownership TXT record
	// must already be in place before this resource is created, see
	// getOwnershipCode in the client and this provider's examples/domain.yaml.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="domainName is immutable"
	DomainName string `json:"domainName"`

	// AllowAccountReset controls whether users on this domain can reset their
	// account via a recovery method. Left unset (nil) leaves PurelyMail's
	// existing/default value untouched rather than forcing it to false.
	// +optional
	AllowAccountReset *bool `json:"allowAccountReset,omitempty"`

	// SymbolicSubaddressing enables "user+tag@domain" style subaddressing. Left
	// unset (nil) leaves PurelyMail's existing/default value untouched rather
	// than forcing it to false.
	// +optional
	SymbolicSubaddressing *bool `json:"symbolicSubaddressing,omitempty"`
}

// DNSSummary reflects PurelyMail's last-checked DNS validation state for a domain.
type DNSSummary struct {
	// PassesMx is true if the domain's MX records are correctly configured.
	PassesMx bool `json:"passesMx,omitempty"`

	// PassesSpf is true if the domain's SPF record is correctly configured.
	PassesSpf bool `json:"passesSpf,omitempty"`

	// PassesDkim is true if the domain's DKIM record is correctly configured.
	PassesDkim bool `json:"passesDkim,omitempty"`

	// PassesDmarc is true if the domain's DMARC record is correctly configured.
	PassesDmarc bool `json:"passesDmarc,omitempty"`
}

// DomainObservation reflects the observed state of a PurelyMail domain.
type DomainObservation struct {
	// IsShared is true if this domain was shared with the account by another
	// PurelyMail user, rather than owned directly.
	IsShared bool `json:"isShared,omitempty"`

	// DNSSummary is PurelyMail's last-checked DNS validation state for this domain.
	DNSSummary DNSSummary `json:"dnsSummary,omitempty"`
}

// A DomainSpec defines the desired state of a PurelymailDomain.
type DomainSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       DomainParameters `json:"forProvider"`
}

// A DomainStatus reflects the observed state of a PurelymailDomain.
type DomainStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          DomainObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="DOMAIN",type="string",JSONPath=".spec.forProvider.domainName"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,purelymail}

// A PurelymailDomain is a managed resource representing a domain registered with PurelyMail.
type PurelymailDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DomainSpec   `json:"spec"`
	Status DomainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PurelymailDomainList contains a list of PurelymailDomain.
type PurelymailDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PurelymailDomain `json:"items"`
}

// GetItems of this PurelymailDomainList, satisfying resource.ManagedList (required
// for cross-resource reference selectors to enumerate candidates).
func (l *PurelymailDomainList) GetItems() []resource.Managed {
	items := make([]resource.Managed, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

// GetCondition of this PurelymailDomain.
func (d *PurelymailDomain) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return d.Status.GetCondition(ct)
}

// SetConditions of this PurelymailDomain.
func (d *PurelymailDomain) SetConditions(c ...xpv1.Condition) {
	d.Status.SetConditions(c...)
}

// GetProviderConfigReference of this PurelymailDomain.
func (d *PurelymailDomain) GetProviderConfigReference() *xpv1.Reference {
	return d.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this PurelymailDomain.
func (d *PurelymailDomain) SetProviderConfigReference(r *xpv1.Reference) {
	d.Spec.ProviderConfigReference = r
}

// GetWriteConnectionSecretToReference of this PurelymailDomain.
func (d *PurelymailDomain) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return d.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this PurelymailDomain.
func (d *PurelymailDomain) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	d.Spec.WriteConnectionSecretToReference = r
}

// GetPublishConnectionDetailsTo of this PurelymailDomain.
func (d *PurelymailDomain) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return d.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this PurelymailDomain.
func (d *PurelymailDomain) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	d.Spec.PublishConnectionDetailsTo = r
}

// GetDeletionPolicy of this PurelymailDomain.
func (d *PurelymailDomain) GetDeletionPolicy() xpv1.DeletionPolicy {
	return d.Spec.DeletionPolicy
}

// SetDeletionPolicy of this PurelymailDomain.
func (d *PurelymailDomain) SetDeletionPolicy(policy xpv1.DeletionPolicy) {
	d.Spec.DeletionPolicy = policy
}

// GetManagementPolicies of this PurelymailDomain.
func (d *PurelymailDomain) GetManagementPolicies() xpv1.ManagementPolicies {
	return d.Spec.ManagementPolicies
}

// SetManagementPolicies of this PurelymailDomain.
func (d *PurelymailDomain) SetManagementPolicies(policies xpv1.ManagementPolicies) {
	d.Spec.ManagementPolicies = policies
}
