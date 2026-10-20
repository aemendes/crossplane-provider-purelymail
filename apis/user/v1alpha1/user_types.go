package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserParameters define the desired state of a PurelyMail user (mailbox).
//
// PurelyMail's own API is asymmetric about how a user is addressed: createUser
// takes a split userName (local part) + domainName, while every other operation
// (getUser, modifyUser, deleteUser) takes the full "local@domain" address as a
// single userName field. This provider stores the split form in spec (mirroring
// createUser) and derives the full address internally wherever the other
// operations need it; the full address is also used as this resource's
// external-name.
type UserParameters struct {
	// Username is the local part of the mailbox address, e.g. "alice" for
	// "alice@example.com". Immutable — PurelyMail does support renaming a user
	// via modifyUser's newUserName, but this provider does not wire that up in
	// v1 to keep the resource model simple (delete/recreate instead).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="username is immutable"
	Username string `json:"username"`

	// DomainName is the domain this user belongs to, e.g. "example.com". Must
	// already exist as a PurelymailDomain (or otherwise be registered with
	// PurelyMail). Immutable for the same reason as Username.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="domainName is immutable"
	DomainName string `json:"domainName"`

	// PasswordSecretRef selects the Secret key holding this user's password.
	// Required at creation. PurelyMail never echoes a user's password back on a
	// read (getUser does not return it), so this provider cannot detect drift
	// between the Secret's current value and what PurelyMail actually has
	// stored — password changes here are only applied at Create time. Rotate by
	// deleting and recreating the resource (or by rotating directly against the
	// PurelyMail API/web UI, out of band of this provider).
	// +kubebuilder:validation:Required
	PasswordSecretRef xpv1.SecretKeySelector `json:"passwordSecretRef"`

	// EnablePasswordReset allows the user to reset their own password via a
	// configured recovery method. Write-only: PurelyMail's getUser does not
	// return this field, so it cannot be observed or diffed after creation.
	// +optional
	EnablePasswordReset *bool `json:"enablePasswordReset,omitempty"`

	// RecoveryEmail is an out-of-band email address usable for account recovery.
	// Write-only, same caveat as EnablePasswordReset.
	// +optional
	RecoveryEmail string `json:"recoveryEmail,omitempty"`

	// RecoveryEmailDescription is a human-readable label for RecoveryEmail.
	// Write-only, same caveat as EnablePasswordReset.
	// +optional
	RecoveryEmailDescription string `json:"recoveryEmailDescription,omitempty"`

	// RecoveryPhone is an out-of-band phone number usable for account recovery.
	// Write-only, same caveat as EnablePasswordReset.
	// +optional
	RecoveryPhone string `json:"recoveryPhone,omitempty"`

	// RecoveryPhoneDescription is a human-readable label for RecoveryPhone.
	// Write-only, same caveat as EnablePasswordReset.
	// +optional
	RecoveryPhoneDescription string `json:"recoveryPhoneDescription,omitempty"`

	// EnableSearchIndexing controls whether this mailbox's contents are
	// indexed for search. Unlike most other optional fields here, PurelyMail's
	// getUser *does* return this, so it is observed and kept in sync by Update.
	// +optional
	EnableSearchIndexing *bool `json:"enableSearchIndexing,omitempty"`

	// SendWelcomeEmail controls whether PurelyMail sends a welcome email when
	// this user is created. Applies only at creation time; there is no
	// equivalent modify field, so this cannot be changed or observed afterward.
	// +optional
	SendWelcomeEmail *bool `json:"sendWelcomeEmail,omitempty"`

	// RequireTwoFactorAuthentication forces this user to configure 2FA.
	// Settable via modifyUser (not createUser) and returned by getUser, so it
	// is observed and kept in sync by Update the same as EnableSearchIndexing.
	// +optional
	RequireTwoFactorAuthentication *bool `json:"requireTwoFactorAuthentication,omitempty"`
}

// ResetMethod is one configured account-recovery method for a PurelyMail user.
type ResetMethod struct {
	// Type of reset method, e.g. "email" or "phone".
	Type string `json:"type,omitempty"`

	// Target is the recovery address/number itself.
	Target string `json:"target,omitempty"`

	// Description is the human-readable label configured for this method.
	Description string `json:"description,omitempty"`

	// AllowMfaReset is true if this method can also be used to reset 2FA.
	AllowMfaReset bool `json:"allowMfaReset,omitempty"`
}

// UserObservation reflects the observed state of a PurelyMail user.
type UserObservation struct {
	// RecoveryEnabled is true if account recovery is currently possible for this
	// user. Read-only: PurelyMail derives it from configured reset methods.
	RecoveryEnabled bool `json:"recoveryEnabled,omitempty"`

	// EnableSpamFiltering reflects whether spam filtering is active for this
	// user. Read-only in this version of the PurelyMail API — there is no known
	// operation that sets it.
	EnableSpamFiltering bool `json:"enableSpamFiltering,omitempty"`

	// ResetMethods lists the account-recovery methods PurelyMail has on file
	// for this user.
	ResetMethods []ResetMethod `json:"resetMethods,omitempty"`
}

// A UserSpec defines the desired state of a PurelymailUser.
type UserSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       UserParameters `json:"forProvider"`
}

// A UserStatus reflects the observed state of a PurelymailUser.
type UserStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          UserObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="DOMAIN",type="string",JSONPath=".spec.forProvider.domainName"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,purelymail}

// A PurelymailUser is a managed resource representing a PurelyMail mailbox.
// Named "PurelymailUser" rather than "User" to avoid colliding with the many
// other CRDs in a typical cluster that also pluralize to "users" — see the
// provider README's "Kind naming" note (same precedent as
// provider-cloudflare-access's AccessApplication).
type PurelymailUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec"`
	Status UserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PurelymailUserList contains a list of PurelymailUser.
type PurelymailUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PurelymailUser `json:"items"`
}

// GetItems of this PurelymailUserList, satisfying resource.ManagedList (required
// for cross-resource reference selectors to enumerate candidates).
func (l *PurelymailUserList) GetItems() []resource.Managed {
	items := make([]resource.Managed, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

// GetCondition of this PurelymailUser.
func (u *PurelymailUser) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return u.Status.GetCondition(ct)
}

// SetConditions of this PurelymailUser.
func (u *PurelymailUser) SetConditions(c ...xpv1.Condition) {
	u.Status.SetConditions(c...)
}

// GetProviderConfigReference of this PurelymailUser.
func (u *PurelymailUser) GetProviderConfigReference() *xpv1.Reference {
	return u.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this PurelymailUser.
func (u *PurelymailUser) SetProviderConfigReference(r *xpv1.Reference) {
	u.Spec.ProviderConfigReference = r
}

// GetWriteConnectionSecretToReference of this PurelymailUser.
func (u *PurelymailUser) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return u.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this PurelymailUser.
func (u *PurelymailUser) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	u.Spec.WriteConnectionSecretToReference = r
}

// GetPublishConnectionDetailsTo of this PurelymailUser.
func (u *PurelymailUser) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return u.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this PurelymailUser.
func (u *PurelymailUser) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	u.Spec.PublishConnectionDetailsTo = r
}

// GetDeletionPolicy of this PurelymailUser.
func (u *PurelymailUser) GetDeletionPolicy() xpv1.DeletionPolicy {
	return u.Spec.DeletionPolicy
}

// SetDeletionPolicy of this PurelymailUser.
func (u *PurelymailUser) SetDeletionPolicy(policy xpv1.DeletionPolicy) {
	u.Spec.DeletionPolicy = policy
}

// GetManagementPolicies of this PurelymailUser.
func (u *PurelymailUser) GetManagementPolicies() xpv1.ManagementPolicies {
	return u.Spec.ManagementPolicies
}

// SetManagementPolicies of this PurelymailUser.
func (u *PurelymailUser) SetManagementPolicies(policies xpv1.ManagementPolicies) {
	u.Spec.ManagementPolicies = policies
}
