// Package purelymail is a minimal REST client for the PurelyMail API
// (https://purelymail.com/docs/index.html).
package purelymail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://purelymail.com"
	requestTimeout = 30 * time.Second

	apiTokenHeader = "Purelymail-Api-Token" // #nosec G101 -- HTTP header name, not a credential
)

// Client is a minimal PurelyMail API client.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiToken   string
}

// NewClient returns a Client authenticated with the supplied PurelyMail API token.
func NewClient(apiToken string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    defaultBaseURL,
		apiToken:   apiToken,
	}
}

// NewClientWithBaseURL returns a Client pointed at a custom base URL, for use in tests.
func NewClientWithBaseURL(apiToken, baseURL string) *Client {
	c := NewClient(apiToken)
	c.baseURL = baseURL
	return c
}

// APIError represents a PurelyMail "type": "error" response. PurelyMail always
// answers with HTTP 200, even for logical errors — the only way to tell an
// error apart from a success is this envelope's "type" field, never the HTTP
// status code.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("purelymail api error (code %q): %s", e.Code, e.Message)
}

const (
	codeInternalError = "internalError"
	codeInvalidToken  = "invalidToken"
)

// IsInvalidToken returns true if err is PurelyMail rejecting the configured API token.
func IsInvalidToken(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == codeInvalidToken
}

// isNotFoundWithPrefix reports whether err is PurelyMail's generic
// "internalError" AND its message starts with the given prefix.
//
// PurelyMail has no dedicated not-found error code: every "unknown X" case
// (user, domain, routing rule) shares the same code "internalError", and is
// only distinguishable by a free-text message. Matching on codeInternalError
// alone would also swallow genuine internal server errors as "not found",
// silently telling a caller a resource is gone when it may just be that the
// API had a bad moment — so every caller below matches a specific message
// prefix instead, and anything that doesn't match propagates as a real error.
func isNotFoundWithPrefix(err error, prefix string) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == codeInternalError && strings.HasPrefix(apiErr.Message, prefix)
}

// IsUserNotFound returns true if err is PurelyMail reporting that a user
// (mailbox) does not exist.
func IsUserNotFound(err error) bool {
	return isNotFoundWithPrefix(err, "Unknown user ")
}

// IsDomainNotFound returns true if err is PurelyMail reporting that a domain
// does not exist.
func IsDomainNotFound(err error) bool {
	return isNotFoundWithPrefix(err, `Unknown domain "`)
}

// IsRoutingRuleNotFound returns true if err is PurelyMail reporting that a
// routing rule does not exist.
func IsRoutingRuleNotFound(err error) bool {
	return isNotFoundWithPrefix(err, "Unknown rule ")
}

// responseEnvelope is PurelyMail's discriminated-union response shape: every
// endpoint answers with either {"type":"success","result":{...}} or
// {"type":"error","code":...,"message":...}, always at HTTP 200.
type responseEnvelope struct {
	Type    string          `json:"type"`
	Result  json.RawMessage `json:"result"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
}

// do issues a POST to https://purelymail.com/api/v0/<verb> with body marshaled
// as the JSON request, and decodes a successful result into out (if non-nil).
// Every PurelyMail operation, including reads and lists, is a POST — there is
// no GET/PUT/PATCH/DELETE anywhere in this API.
func (c *Client) do(ctx context.Context, verb string, body, out any) error {
	if body == nil {
		body = struct{}{}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v0/"+verb, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set(apiTokenHeader, c.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best effort close

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// PurelyMail itself always answers 200, so a non-200 here means something
	// above the API (a proxy, a load balancer) intervened rather than PurelyMail
	// itself — there's no envelope to decode in that case.
	if resp.StatusCode != http.StatusOK {
		const maxSnippet = 200
		snippet := respBody
		if len(snippet) > maxSnippet {
			snippet = snippet[:maxSnippet]
		}
		return fmt.Errorf("unexpected HTTP status %d from PurelyMail: %q", resp.StatusCode, snippet)
	}

	var env responseEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return fmt.Errorf("decode response envelope: %w", err)
	}

	switch env.Type {
	case "error":
		return &APIError{Code: env.Code, Message: env.Message}
	case "success":
		if out == nil || len(env.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unexpected response type %q from PurelyMail", env.Type)
	}
}

// DNSSummary is PurelyMail's last-checked DNS validation state for a domain.
type DNSSummary struct {
	PassesMx    bool `json:"passesMx"`
	PassesSpf   bool `json:"passesSpf"`
	PassesDkim  bool `json:"passesDkim"`
	PassesDmarc bool `json:"passesDmarc"`
}

// ApiDomainInfo mirrors the subset of PurelyMail's domain object this provider manages.
type ApiDomainInfo struct {
	Name                  string     `json:"name"`
	AllowAccountReset     bool       `json:"allowAccountReset"`
	SymbolicSubaddressing bool       `json:"symbolicSubaddressing"`
	IsShared              bool       `json:"isShared"`
	DNSSummary            DNSSummary `json:"dnsSummary"`
}

type addDomainRequest struct {
	DomainName string `json:"domainName"`
}

// AddDomain registers a new domain with PurelyMail. The domain's ownership TXT
// record must already be in place, or PurelyMail rejects the call.
func (c *Client) AddDomain(ctx context.Context, domainName string) error {
	return c.do(ctx, "addDomain", addDomainRequest{DomainName: domainName}, nil)
}

type listDomainsRequest struct {
	IncludeShared bool `json:"includeShared"`
}

type listDomainsResponse struct {
	Domains []ApiDomainInfo `json:"domains"`
}

// ListDomains lists every domain on the account. There is no getDomain
// operation in PurelyMail's API — looking up a single domain means listing
// and filtering client-side.
func (c *Client) ListDomains(ctx context.Context, includeShared bool) ([]ApiDomainInfo, error) {
	var resp listDomainsResponse
	if err := c.do(ctx, "listDomains", listDomainsRequest{IncludeShared: includeShared}, &resp); err != nil {
		return nil, err
	}
	return resp.Domains, nil
}

// UpdateDomainSettingsInput is the partial-update payload for updateDomainSettings.
// AllowAccountReset/SymbolicSubaddressing are only changed if non-nil — PurelyMail
// leaves omitted fields untouched rather than resetting them.
type UpdateDomainSettingsInput struct {
	Name                  string
	AllowAccountReset     *bool
	SymbolicSubaddressing *bool
	// RecheckDNS asks PurelyMail to re-validate the domain's DNS records
	// (MX/SPF/DKIM/DMARC) as part of this call. Not exposed on the
	// PurelymailDomain spec in v1 — always sent as false so routine reconciles
	// don't force an unrequested DNS re-check on every drift-correcting update.
	RecheckDNS bool
}

type updateDomainSettingsRequest struct {
	Name                  string `json:"name"`
	AllowAccountReset     *bool  `json:"allowAccountReset,omitempty"`
	SymbolicSubaddressing *bool  `json:"symbolicSubaddressing,omitempty"`
	RecheckDNS            bool   `json:"recheckDns"`
}

// UpdateDomainSettings applies a partial update to an existing domain's settings.
func (c *Client) UpdateDomainSettings(ctx context.Context, in UpdateDomainSettingsInput) error {
	req := updateDomainSettingsRequest{
		Name:                  in.Name,
		AllowAccountReset:     in.AllowAccountReset,
		SymbolicSubaddressing: in.SymbolicSubaddressing,
		RecheckDNS:            in.RecheckDNS,
	}
	return c.do(ctx, "updateDomainSettings", req, nil)
}

type deleteDomainRequest struct {
	Name string `json:"name"`
}

// DeleteDomain removes a domain from the account.
func (c *Client) DeleteDomain(ctx context.Context, name string) error {
	return c.do(ctx, "deleteDomain", deleteDomainRequest{Name: name}, nil)
}

type getOwnershipCodeResponse struct {
	Code string `json:"code"`
}

// GetOwnershipCode returns the account-level ownership code that must be
// published as a DNS TXT record before a new domain can be added with
// AddDomain. Account-level, not per-domain, so this is a plain client helper
// rather than something surfaced on any one managed resource's status.
func (c *Client) GetOwnershipCode(ctx context.Context) (string, error) {
	var resp getOwnershipCodeResponse
	if err := c.do(ctx, "getOwnershipCode", nil, &resp); err != nil {
		return "", err
	}
	return resp.Code, nil
}

// CreateUserInput is the payload for createUser. Unlike every other user
// operation, createUser splits the address into UserName (local part only)
// and DomainName rather than taking a single full address.
type CreateUserInput struct {
	UserName                 string
	DomainName               string
	Password                 string
	EnablePasswordReset      *bool
	RecoveryEmail            string
	RecoveryEmailDescription string
	RecoveryPhone            string
	RecoveryPhoneDescription string
	EnableSearchIndexing     *bool
	SendWelcomeEmail         *bool
}

type createUserRequest struct {
	UserName                 string `json:"userName"`
	DomainName               string `json:"domainName"`
	Password                 string `json:"password"`
	EnablePasswordReset      *bool  `json:"enablePasswordReset,omitempty"`
	RecoveryEmail            string `json:"recoveryEmail,omitempty"`
	RecoveryEmailDescription string `json:"recoveryEmailDescription,omitempty"`
	RecoveryPhone            string `json:"recoveryPhone,omitempty"`
	RecoveryPhoneDescription string `json:"recoveryPhoneDescription,omitempty"`
	EnableSearchIndexing     *bool  `json:"enableSearchIndexing,omitempty"`
	SendWelcomeEmail         *bool  `json:"sendWelcomeEmail,omitempty"`
}

// CreateUser creates a new mailbox.
func (c *Client) CreateUser(ctx context.Context, in CreateUserInput) error {
	req := createUserRequest{
		UserName:                 in.UserName,
		DomainName:               in.DomainName,
		Password:                 in.Password,
		EnablePasswordReset:      in.EnablePasswordReset,
		RecoveryEmail:            in.RecoveryEmail,
		RecoveryEmailDescription: in.RecoveryEmailDescription,
		RecoveryPhone:            in.RecoveryPhone,
		RecoveryPhoneDescription: in.RecoveryPhoneDescription,
		EnableSearchIndexing:     in.EnableSearchIndexing,
		SendWelcomeEmail:         in.SendWelcomeEmail,
	}
	return c.do(ctx, "createUser", req, nil)
}

// ResetMethod is one configured account-recovery method for a user.
type ResetMethod struct {
	Type          string `json:"type"`
	Target        string `json:"target"`
	Description   string `json:"description"`
	AllowMfaReset bool   `json:"allowMfaReset"`
}

// UserInfo mirrors getUser's response. Note what's absent: password,
// sendWelcomeEmail, and the recovery* fields are write-only in this API and
// never echoed back here.
type UserInfo struct {
	EnableSearchIndexing           bool          `json:"enableSearchIndexing"`
	RecoveryEnabled                bool          `json:"recoveryEnabled"`
	RequireTwoFactorAuthentication bool          `json:"requireTwoFactorAuthentication"`
	EnableSpamFiltering            bool          `json:"enableSpamFiltering"`
	ResetMethods                   []ResetMethod `json:"resetMethods"`
}

type getUserRequest struct {
	UserName string `json:"userName"`
}

// GetUser fetches a mailbox by its full "local@domain" address. Returns an
// *APIError matched by IsUserNotFound if it does not exist.
func (c *Client) GetUser(ctx context.Context, userName string) (*UserInfo, error) {
	var resp UserInfo
	if err := c.do(ctx, "getUser", getUserRequest{UserName: userName}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ModifyUserInput is the payload for modifyUser. UserName (the full address)
// is required; every other field is applied only if non-empty/non-nil.
type ModifyUserInput struct {
	UserName                       string
	NewUserName                    string
	NewPassword                    string
	EnableSearchIndexing           *bool
	EnablePasswordReset            *bool
	RequireTwoFactorAuthentication *bool
}

type modifyUserRequest struct {
	UserName                       string `json:"userName"`
	NewUserName                    string `json:"newUserName,omitempty"`
	NewPassword                    string `json:"newPassword,omitempty"`
	EnableSearchIndexing           *bool  `json:"enableSearchIndexing,omitempty"`
	EnablePasswordReset            *bool  `json:"enablePasswordReset,omitempty"`
	RequireTwoFactorAuthentication *bool  `json:"requireTwoFactorAuthentication,omitempty"`
}

// ModifyUser updates the mutable fields of an existing mailbox.
func (c *Client) ModifyUser(ctx context.Context, in ModifyUserInput) error {
	req := modifyUserRequest{
		UserName:                       in.UserName,
		NewUserName:                    in.NewUserName,
		NewPassword:                    in.NewPassword,
		EnableSearchIndexing:           in.EnableSearchIndexing,
		EnablePasswordReset:            in.EnablePasswordReset,
		RequireTwoFactorAuthentication: in.RequireTwoFactorAuthentication,
	}
	return c.do(ctx, "modifyUser", req, nil)
}

type listUserResponse struct {
	Users []string `json:"users"`
}

// ListUsers lists every mailbox address on the account. Flat strings, no
// pagination (PurelyMail reportedly caps this around 1000 users).
func (c *Client) ListUsers(ctx context.Context) ([]string, error) {
	var resp listUserResponse
	if err := c.do(ctx, "listUser", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Users, nil
}

type deleteUserRequest struct {
	UserName string `json:"userName"`
}

// DeleteUser deletes a mailbox by its full "local@domain" address.
func (c *Client) DeleteUser(ctx context.Context, userName string) error {
	return c.do(ctx, "deleteUser", deleteUserRequest{UserName: userName}, nil)
}

// RoutingRule mirrors a PurelyMail routing rule, including the id createRoutingRule itself never returns.
type RoutingRule struct {
	ID              int64    `json:"id"`
	DomainName      string   `json:"domainName"`
	Prefix          bool     `json:"prefix"`
	MatchUser       string   `json:"matchUser"`
	TargetAddresses []string `json:"targetAddresses"`
	Catchall        bool     `json:"catchall"`
}

// CreateRoutingRuleInput is the payload for createRoutingRule.
type CreateRoutingRuleInput struct {
	DomainName      string
	Prefix          bool
	MatchUser       string
	TargetAddresses []string
	Catchall        bool
}

type createRoutingRuleRequest struct {
	DomainName      string   `json:"domainName"`
	Prefix          bool     `json:"prefix"`
	MatchUser       string   `json:"matchUser"`
	TargetAddresses []string `json:"targetAddresses"`
	Catchall        bool     `json:"catchall"`
}

// CreateRoutingRule creates a new routing rule. The response never includes
// the new rule's id — callers must recover it via ListRoutingRules, matching
// on the full field tuple (see internal/controller/routingrule).
func (c *Client) CreateRoutingRule(ctx context.Context, in CreateRoutingRuleInput) error {
	req := createRoutingRuleRequest{
		DomainName:      in.DomainName,
		Prefix:          in.Prefix,
		MatchUser:       in.MatchUser,
		TargetAddresses: in.TargetAddresses,
		Catchall:        in.Catchall,
	}
	return c.do(ctx, "createRoutingRule", req, nil)
}

type listRoutingRulesResponse struct {
	Rules []RoutingRule `json:"rules"`
}

// ListRoutingRules lists every routing rule on the account (account-wide, not
// filterable server-side by domain or anything else).
func (c *Client) ListRoutingRules(ctx context.Context) ([]RoutingRule, error) {
	var resp listRoutingRulesResponse
	if err := c.do(ctx, "listRoutingRules", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Rules, nil
}

type deleteRoutingRuleRequest struct {
	RoutingRuleID int64 `json:"routingRuleId"`
}

// DeleteRoutingRule deletes a routing rule by id.
func (c *Client) DeleteRoutingRule(ctx context.Context, id int64) error {
	return c.do(ctx, "deleteRoutingRule", deleteRoutingRuleRequest{RoutingRuleID: id}, nil)
}

// FormatRoutingRuleID and ParseRoutingRuleID centralize the string<->int64
// conversion used to store a routing rule's id as this resource's external-name.

// FormatRoutingRuleID renders a routing rule id as an external-name string.
func FormatRoutingRuleID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// ParseRoutingRuleID parses an external-name string back into a routing rule id.
func ParseRoutingRuleID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
