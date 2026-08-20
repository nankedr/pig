package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// AuthType is the published credential discriminator. A stored credential is
// exactly one of these; the union stays closed and its codec rejects unknown
// or null discriminators.
type AuthType string

const (
	// AuthTypeAPIKey tags a stored api-key credential.
	AuthTypeAPIKey AuthType = "api_key"
	// AuthTypeOAuth tags a stored canonical OAuth credential.
	AuthTypeOAuth AuthType = "oauth"
)

// Credential is the closed set of stored credentials — one type-tagged
// credential per provider, the shape of today's auth.json. Callers type-switch
// over the concrete variants; MarshalCredential and UnmarshalCredential are the
// authoritative wire codec.
type Credential interface {
	credential()
	// CredentialType reports the published discriminator of the concrete
	// variant. A forged or empty discriminator is rejected by the codec.
	CredentialType() AuthType
}

// APIKeyCredential is a stored api-key credential. Key is absent when the
// provider resolves api keys purely from ambient sources. Env holds
// provider-scoped environment/config values such as Cloudflare account/gateway
// ids and is preserved verbatim.
type APIKeyCredential struct {
	Type AuthType         `json:"type"`
	Key  Optional[string] `json:"key,omitzero"`
	Env  ProviderEnv      `json:"env,omitempty"`
}

func (APIKeyCredential) credential() {}

// CredentialType returns the stored discriminator.
func (c APIKeyCredential) CredentialType() AuthType { return c.Type }

// OAuthCredentials is the OAuth token data returned by extension compatibility
// flows. Beyond the three known fields it preserves any additional provider
// fields losslessly (upstream `[key: string]: unknown`) so a round-trip never
// drops provider-specific token metadata.
type OAuthCredentials struct {
	Refresh string
	Access  string
	Expires int64
	// Extra retains additional provider fields as exact JSON bytes. Keys never
	// include refresh, access or expires.
	Extra map[string]json.RawMessage
}

// oauthCredentialsReserved are the keys OAuthCredentials owns; they never leak
// into Extra.
var oauthCredentialsReserved = map[string]bool{"refresh": true, "access": true, "expires": true}

// oauthCredentialReserved additionally reserves the credential discriminator.
var oauthCredentialReserved = map[string]bool{"refresh": true, "access": true, "expires": true, "type": true}

// fieldMap renders the known OAuth fields and preserved extras into a single
// object. Known fields always win over any colliding extra key.
func (c OAuthCredentials) fieldMap(reserved map[string]bool) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage, len(c.Extra)+3)
	for key, raw := range c.Extra {
		if reserved[key] {
			continue
		}
		fields[key] = append(json.RawMessage(nil), raw...)
	}
	refresh, err := json.Marshal(c.Refresh)
	if err != nil {
		return nil, err
	}
	access, err := json.Marshal(c.Access)
	if err != nil {
		return nil, err
	}
	expires, err := json.Marshal(c.Expires)
	if err != nil {
		return nil, err
	}
	fields["refresh"] = refresh
	fields["access"] = access
	fields["expires"] = expires
	return fields, nil
}

// MarshalJSON encodes the OAuth token data with its preserved extras.
func (c OAuthCredentials) MarshalJSON() ([]byte, error) {
	fields, err := c.fieldMap(oauthCredentialsReserved)
	if err != nil {
		return nil, newCodecError("oauth credentials", "", err)
	}
	return json.Marshal(fields)
}

// UnmarshalJSON decodes the OAuth token data, retaining unknown fields in Extra.
func (c *OAuthCredentials) UnmarshalJSON(data []byte) error {
	decoded, err := decodeOAuthCredentials(data, oauthCredentialsReserved)
	if err != nil {
		return newCodecError("oauth credentials", "", err)
	}
	*c = decoded
	return nil
}

func decodeOAuthCredentials(data []byte, reserved map[string]bool) (OAuthCredentials, error) {
	fields, err := decodeWireObject(data)
	if err != nil {
		return OAuthCredentials{}, err
	}
	if err := requireWireString(fields, "refresh"); err != nil {
		return OAuthCredentials{}, err
	}
	if err := requireWireString(fields, "access"); err != nil {
		return OAuthCredentials{}, err
	}
	if err := requireWireNumber(fields, "expires"); err != nil {
		return OAuthCredentials{}, err
	}

	var known struct {
		Refresh string `json:"refresh"`
		Access  string `json:"access"`
		Expires int64  `json:"expires"`
	}
	if err := json.Unmarshal(data, &known); err != nil {
		return OAuthCredentials{}, err
	}

	var extra map[string]json.RawMessage
	for key, raw := range fields {
		if reserved[key] {
			continue
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[key] = append(json.RawMessage(nil), raw...)
	}
	return OAuthCredentials{Refresh: known.Refresh, Access: known.Access, Expires: known.Expires, Extra: extra}, nil
}

// OAuthCredential is the stored canonical OAuth credential — OAuth token data
// tagged with the "oauth" discriminator.
type OAuthCredential struct {
	OAuthCredentials
	Type AuthType
}

func (OAuthCredential) credential() {}

// CredentialType returns the stored discriminator.
func (c OAuthCredential) CredentialType() AuthType { return c.Type }

// MarshalJSON encodes the credential form: OAuth token data plus its
// discriminator. The type key wins over any colliding preserved extra.
func (c OAuthCredential) MarshalJSON() ([]byte, error) {
	fields, err := c.OAuthCredentials.fieldMap(oauthCredentialReserved)
	if err != nil {
		return nil, newCodecError("oauth credential", string(c.Type), err)
	}
	discriminator, err := json.Marshal(c.Type)
	if err != nil {
		return nil, newCodecError("oauth credential", string(c.Type), err)
	}
	fields["type"] = discriminator
	return json.Marshal(fields)
}

// UnmarshalJSON decodes the credential form, retaining unknown fields in Extra
// and requiring the "oauth" discriminator.
func (c *OAuthCredential) UnmarshalJSON(data []byte) error {
	fields, err := decodeWireObject(data)
	if err != nil {
		return newCodecError("oauth credential", "", err)
	}
	if err := requireWireString(fields, "type"); err != nil {
		return newCodecError("oauth credential", "", err)
	}
	var envelope struct {
		Type AuthType `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return newCodecError("oauth credential", "", err)
	}
	if envelope.Type != AuthTypeOAuth {
		return newCodecError("oauth credential", string(envelope.Type), fmt.Errorf("discriminator must be %q", AuthTypeOAuth))
	}
	decoded, err := decodeOAuthCredentials(data, oauthCredentialReserved)
	if err != nil {
		return newCodecError("oauth credential", string(envelope.Type), err)
	}
	*c = OAuthCredential{OAuthCredentials: decoded, Type: envelope.Type}
	return nil
}

// MarshalCredential encodes one member of the closed credential union after
// checking that its concrete variant and published discriminator agree. A
// typed-nil pointer or mismatched discriminator is rejected rather than encoded.
func MarshalCredential(credential Credential) ([]byte, error) {
	want, ok := credentialVariantType(credential)
	if !ok || isNilClosedUnion(credential) {
		return nil, newCodecError("credential", "", fmt.Errorf("unsupported concrete type %T", credential))
	}
	if got := credential.CredentialType(); got != want {
		return nil, newCodecError("credential", string(got), fmt.Errorf("%T requires %q", credential, want))
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		if errors.Is(err, ErrCodec) {
			return nil, err
		}
		return nil, newCodecError("credential", string(want), err)
	}
	return encoded, nil
}

// UnmarshalCredential decodes one member of the closed credential union.
// Unknown discriminators and null are rejected instead of being retained as an
// open value.
func UnmarshalCredential(data []byte) (Credential, error) {
	fields, err := decodeWireObject(data)
	if err != nil {
		return nil, newCodecError("credential", "", err)
	}
	if err := requireWireString(fields, "type"); err != nil {
		return nil, newCodecError("credential", "", err)
	}
	var envelope struct {
		Type AuthType `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, newCodecError("credential", "", err)
	}

	switch envelope.Type {
	case AuthTypeAPIKey:
		var credential APIKeyCredential
		if err := json.Unmarshal(data, &credential); err != nil {
			return nil, newCodecError("credential", string(envelope.Type), err)
		}
		credential.Type = envelope.Type
		return credential, nil
	case AuthTypeOAuth:
		var credential OAuthCredential
		if err := json.Unmarshal(data, &credential); err != nil {
			return nil, wrapCodecError("credential", string(envelope.Type), err)
		}
		return credential, nil
	default:
		return nil, newCodecError("credential", string(envelope.Type), nil)
	}
}

func credentialVariantType(credential Credential) (AuthType, bool) {
	switch credential.(type) {
	case APIKeyCredential, *APIKeyCredential:
		return AuthTypeAPIKey, true
	case OAuthCredential, *OAuthCredential:
		return AuthTypeOAuth, true
	default:
		return "", false
	}
}

// CredentialInfo is non-secret credential metadata for account/status
// enumeration. It never carries the secret itself.
type CredentialInfo struct {
	ProviderID ProviderID `json:"providerId"`
	Type       AuthType   `json:"type"`
}

// AuthOperationOptions carries optional per-operation settings for credential
// and auth operations. Cancellation — the sole upstream option — is expressed
// in Go through the context.Context passed to each operation, so this struct is
// currently empty and reserved as the seam for future operation-scoped options.
type AuthOperationOptions struct{}

// CredentialModifyFunc is the serialized read-modify-write callback used by
// CredentialStore.Modify. It receives the current credential (nil when none is
// stored) and returns the replacement credential, nil to leave the entry
// unchanged, or an error that aborts the write and propagates to the caller.
type CredentialModifyFunc func(ctx context.Context, current Credential) (Credential, error)

// CredentialStore is app-owned credential storage keyed by provider id, one
// credential per provider. Modify is the only write path, so every mutation is
// a serialized read-modify-write; OAuth refresh runs inside Modify so
// concurrent requests cannot double-refresh a rotated token.
//
// Error semantics mirror the upstream contract: Read yields a nil credential
// for a missing entry, and cancellation via ctx is reported as the context
// error.
type CredentialStore interface {
	// Read returns the stored credential, possibly expired, or nil when none is
	// stored. Display/status use; resolved request auth comes from auth
	// resolution.
	Read(ctx context.Context, providerID ProviderID, options AuthOperationOptions) (Credential, error)

	// List returns stored credential metadata without resolving or exposing
	// secrets. Implementations must not execute configured api-key commands.
	List(ctx context.Context, options AuthOperationOptions) ([]CredentialInfo, error)

	// Modify runs the only write path. fn sees the current credential because
	// correct writes (refresh, login-during-refresh) depend on it. Writes are
	// mutually exclusive per provider id. It returns the post-write credential.
	Modify(ctx context.Context, providerID ProviderID, fn CredentialModifyFunc, options AuthOperationOptions) (Credential, error)

	// Delete removes a credential (logout). It is serialized against Modify.
	Delete(ctx context.Context, providerID ProviderID, options AuthOperationOptions) error
}

// InMemoryCredentialStore is the default in-memory credential store. Apps
// inject persistent stores. It is live (no I/O): reads and lists observe a
// consistent snapshot, while writes and deletes are serialized per provider id
// through a cancellable lock so a rotated token cannot be double-written.
type InMemoryCredentialStore struct {
	mu          sync.Mutex
	credentials map[ProviderID]Credential
	locks       map[ProviderID]chan struct{}
}

// NewInMemoryCredentialStore returns an empty in-memory credential store.
func NewInMemoryCredentialStore() *InMemoryCredentialStore {
	return &InMemoryCredentialStore{
		credentials: make(map[ProviderID]Credential),
		locks:       make(map[ProviderID]chan struct{}),
	}
}

// acquire takes the per-provider write lock, honoring ctx cancellation. The
// lock is a one-slot channel so serialization is cross-goroutine and abortable.
func (s *InMemoryCredentialStore) acquire(ctx context.Context, providerID ProviderID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	lock, ok := s.locks[providerID]
	if !ok {
		lock = make(chan struct{}, 1)
		s.locks[providerID] = lock
	}
	s.mu.Unlock()

	select {
	case lock <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *InMemoryCredentialStore) release(providerID ProviderID) {
	s.mu.Lock()
	lock := s.locks[providerID]
	s.mu.Unlock()
	<-lock
}

// Read returns the stored credential or nil when none is stored.
func (s *InMemoryCredentialStore) Read(ctx context.Context, providerID ProviderID, _ AuthOperationOptions) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.credentials[providerID], nil
}

// List returns stored credential metadata sorted by provider id for
// deterministic enumeration.
func (s *InMemoryCredentialStore) List(ctx context.Context, _ AuthOperationOptions) ([]CredentialInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	infos := make([]CredentialInfo, 0, len(s.credentials))
	for providerID, credential := range s.credentials {
		infos = append(infos, CredentialInfo{ProviderID: providerID, Type: credential.CredentialType()})
	}
	s.mu.Unlock()
	sort.Slice(infos, func(i, j int) bool { return infos[i].ProviderID < infos[j].ProviderID })
	return infos, nil
}

// Modify runs the serialized read-modify-write for one provider id.
func (s *InMemoryCredentialStore) Modify(ctx context.Context, providerID ProviderID, fn CredentialModifyFunc, _ AuthOperationOptions) (Credential, error) {
	if fn == nil {
		return nil, fmt.Errorf("ai: credential modify function must not be nil")
	}
	if err := s.acquire(ctx, providerID); err != nil {
		return nil, err
	}
	defer s.release(providerID)

	s.mu.Lock()
	current := s.credentials[providerID]
	s.mu.Unlock()

	next, err := fn(ctx, current)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if next != nil {
		s.mu.Lock()
		s.credentials[providerID] = next
		s.mu.Unlock()
		return next, nil
	}
	return current, nil
}

// Delete removes a credential, serialized against Modify for the same provider.
func (s *InMemoryCredentialStore) Delete(ctx context.Context, providerID ProviderID, _ AuthOperationOptions) error {
	if err := s.acquire(ctx, providerID); err != nil {
		return err
	}
	defer s.release(providerID)

	s.mu.Lock()
	delete(s.credentials, providerID)
	s.mu.Unlock()
	return nil
}
