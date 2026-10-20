package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoutingRuleParameters define the desired state of a PurelyMail routing rule.
//
// PurelyMail has no update operation for a routing rule at all, and
// createRoutingRule does not return the new rule's id in its response — the
// only way to learn a rule's id is to list every rule on the account
// (listRoutingRules has no server-side filter) and match on the full field
// tuple below. This provider's controller does that id-recovery dance on
// Create, and implements Update as delete-then-create against a new id (see
// internal/controller/routingrule).
type RoutingRuleParameters struct {
	// DomainName is the domain this rule applies to.
	// +kubebuilder:validation:Required
	DomainName string `json:"domainName"`

	// Prefix, if true, makes MatchUser a prefix match rather than an exact
	// local-part match.
	// +optional
	Prefix bool `json:"prefix,omitempty"`

	// MatchUser is the local-part (or prefix, if Prefix is true) to match
	// incoming mail against. Ignored by PurelyMail when Catchall is true.
	// +optional
	MatchUser string `json:"matchUser,omitempty"`

	// TargetAddresses is where matching mail is routed.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	TargetAddresses []string `json:"targetAddresses"`

	// Catchall, if true, matches every address on DomainName not otherwise
	// claimed by a real mailbox or a more specific rule.
	// +optional
	Catchall bool `json:"catchall,omitempty"`
}

// RoutingRuleObservation reflects the observed state of a PurelyMail routing rule.
type RoutingRuleObservation struct {
	// ID is PurelyMail's numeric id for this rule, duplicated here (it's also
	// the external-name) purely for convenience/visibility via `kubectl get -o
	// wide` and similar.
	ID int64 `json:"id,omitempty"`
}

// A RoutingRuleSpec defines the desired state of a PurelymailRoutingRule.
type RoutingRuleSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       RoutingRuleParameters `json:"forProvider"`
}

// A RoutingRuleStatus reflects the observed state of a PurelymailRoutingRule.
type RoutingRuleStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          RoutingRuleObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="DOMAIN",type="string",JSONPath=".spec.forProvider.domainName"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,purelymail}

// A PurelymailRoutingRule is a managed resource representing a PurelyMail mail
// routing rule.
type PurelymailRoutingRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoutingRuleSpec   `json:"spec"`
	Status RoutingRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PurelymailRoutingRuleList contains a list of PurelymailRoutingRule.
type PurelymailRoutingRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PurelymailRoutingRule `json:"items"`
}

// GetItems of this PurelymailRoutingRuleList, satisfying resource.ManagedList
// (required for cross-resource reference selectors to enumerate candidates).
func (l *PurelymailRoutingRuleList) GetItems() []resource.Managed {
	items := make([]resource.Managed, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

// GetCondition of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return r.Status.GetCondition(ct)
}

// SetConditions of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) SetConditions(c ...xpv1.Condition) {
	r.Status.SetConditions(c...)
}

// GetProviderConfigReference of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) GetProviderConfigReference() *xpv1.Reference {
	return r.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) SetProviderConfigReference(ref *xpv1.Reference) {
	r.Spec.ProviderConfigReference = ref
}

// GetWriteConnectionSecretToReference of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return r.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) SetWriteConnectionSecretToReference(s *xpv1.SecretReference) {
	r.Spec.WriteConnectionSecretToReference = s
}

// GetPublishConnectionDetailsTo of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return r.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) SetPublishConnectionDetailsTo(p *xpv1.PublishConnectionDetailsTo) {
	r.Spec.PublishConnectionDetailsTo = p
}

// GetDeletionPolicy of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) GetDeletionPolicy() xpv1.DeletionPolicy {
	return r.Spec.DeletionPolicy
}

// SetDeletionPolicy of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) SetDeletionPolicy(policy xpv1.DeletionPolicy) {
	r.Spec.DeletionPolicy = policy
}

// GetManagementPolicies of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) GetManagementPolicies() xpv1.ManagementPolicies {
	return r.Spec.ManagementPolicies
}

// SetManagementPolicies of this PurelymailRoutingRule.
func (r *PurelymailRoutingRule) SetManagementPolicies(policies xpv1.ManagementPolicies) {
	r.Spec.ManagementPolicies = policies
}
