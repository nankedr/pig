package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/nankedr/pig/ai"
)

// Compile-time surface parity: the credential union, its metadata, and the
// store contract must expose exactly the shapes the upstream auth.json codec
// and CredentialStore promise. A dropped variant or method fails to build.
var (
	_ ai.Credential      = ai.APIKeyCredential{}
	_ ai.Credential      = ai.OAuthCredential{}
	_ ai.CredentialStore = (*ai.InMemoryCredentialStore)(nil)
)

func TestCredentialUnionRoundTripsEveryVariant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		credential ai.Credential
		wantType   ai.AuthType
	}{
		{
			name:       "api key with stored key and provider env",
			credential: ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("sk-live"), Env: ai.ProviderEnv{"CLOUDFLARE_ACCOUNT_ID": "acc-1"}},
			wantType:   ai.AuthTypeAPIKey,
		},
		{
			name:       "ambient-only api key omits the key",
			credential: ai.APIKeyCredential{Type: ai.AuthTypeAPIKey},
			wantType:   ai.AuthTypeAPIKey,
		},
		{
			name: "oauth credential",
			credential: ai.OAuthCredential{
				OAuthCredentials: ai.OAuthCredentials{Refresh: "r", Access: "a", Expires: 1730000000000},
				Type:             ai.AuthTypeOAuth,
			},
			wantType: ai.AuthTypeOAuth,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := ai.MarshalCredential(test.credential)
			if err != nil {
				t.Fatalf("MarshalCredential() error = %v", err)
			}
			decoded, err := ai.UnmarshalCredential(encoded)
			if err != nil {
				t.Fatalf("UnmarshalCredential() error = %v", err)
			}
			if decoded.CredentialType() != test.wantType {
				t.Fatalf("CredentialType() = %q, want %q", decoded.CredentialType(), test.wantType)
			}
			if !reflect.DeepEqual(decoded, test.credential) {
				t.Fatalf("round-trip credential = %#v, want %#v", decoded, test.credential)
			}
		})
	}
}

func TestOAuthCredentialPreservesUnknownProviderFieldsLosslessly(t *testing.T) {
	t.Parallel()

	// A large integer and nested object would be corrupted if extras were
	// decoded through map[string]any and re-encoded instead of retained raw.
	input := []byte(`{"type":"oauth","refresh":"r","access":"a","expires":1730000000000,` +
		`"scope":"read write","account":{"id":9007199254740993},"raw":[1,2,3]}`)

	credential, err := ai.UnmarshalCredential(input)
	if err != nil {
		t.Fatalf("UnmarshalCredential() error = %v", err)
	}
	oauth, ok := credential.(ai.OAuthCredential)
	if !ok {
		t.Fatalf("decoded credential = %T, want ai.OAuthCredential", credential)
	}
	for _, reserved := range []string{"type", "refresh", "access", "expires"} {
		if _, leaked := oauth.Extra[reserved]; leaked {
			t.Fatalf("reserved key %q leaked into Extra %v", reserved, oauth.Extra)
		}
	}
	if got := string(oauth.Extra["account"]); got != `{"id":9007199254740993}` {
		t.Fatalf("account extra = %s, want exact bytes with the large integer intact", got)
	}

	encoded, err := ai.MarshalCredential(oauth)
	if err != nil {
		t.Fatalf("MarshalCredential() error = %v", err)
	}
	var reencoded, original map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &reencoded); err != nil {
		t.Fatalf("json.Unmarshal(reencoded) error = %v", err)
	}
	if err := json.Unmarshal(input, &original); err != nil {
		t.Fatalf("json.Unmarshal(input) error = %v", err)
	}
	if len(reencoded) != len(original) {
		t.Fatalf("re-encoded field set = %v, want the same keys as %v", reencoded, original)
	}
	if string(reencoded["account"]) != `{"id":9007199254740993}` {
		t.Fatalf("re-encoded account = %s, want the large integer preserved", reencoded["account"])
	}
}

func TestOAuthCredentialsBareFormRoundTripsWithoutDiscriminator(t *testing.T) {
	t.Parallel()

	input := []byte(`{"refresh":"r","access":"a","expires":10,"idToken":"jwt"}`)
	var credentials ai.OAuthCredentials
	if err := json.Unmarshal(input, &credentials); err != nil {
		t.Fatalf("json.Unmarshal(OAuthCredentials) error = %v", err)
	}
	if credentials.Refresh != "r" || credentials.Access != "a" || credentials.Expires != 10 {
		t.Fatalf("decoded OAuthCredentials = %#v", credentials)
	}
	if got := string(credentials.Extra["idToken"]); got != `"jwt"` {
		t.Fatalf("idToken extra = %s, want %q", got, `"jwt"`)
	}

	encoded, err := json.Marshal(credentials)
	if err != nil {
		t.Fatalf("json.Marshal(OAuthCredentials) error = %v", err)
	}
	if bytes := string(encoded); !json.Valid(encoded) || bytes == "" {
		t.Fatalf("re-encoded OAuthCredentials invalid: %s", bytes)
	}
	var reencoded map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &reencoded); err != nil {
		t.Fatalf("json.Unmarshal(reencoded) error = %v", err)
	}
	if _, hasType := reencoded["type"]; hasType {
		t.Fatalf("bare OAuthCredentials must not carry a discriminator: %s", encoded)
	}
	if string(reencoded["idToken"]) != `"jwt"` {
		t.Fatalf("re-encoded idToken = %s, want preserved", reencoded["idToken"])
	}
}

func TestMarshalCredentialRejectsForgedOrNilVariants(t *testing.T) {
	t.Parallel()

	// A concrete variant whose discriminator disagrees with its type is rejected
	// rather than silently encoded under the wrong tag.
	if _, err := ai.MarshalCredential(ai.APIKeyCredential{Type: ai.AuthTypeOAuth}); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("MarshalCredential(mismatched discriminator) error = %v, want ErrCodec", err)
	}

	var typedNil *ai.APIKeyCredential
	if _, err := ai.MarshalCredential(typedNil); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("MarshalCredential(typed nil) error = %v, want ErrCodec", err)
	}

	if _, err := ai.MarshalCredential(nil); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("MarshalCredential(nil) error = %v, want ErrCodec", err)
	}
}

func TestUnmarshalCredentialRejectsUnknownNullAndMissingDiscriminators(t *testing.T) {
	t.Parallel()

	inputs := []string{
		`{"type":"future"}`,
		`{"type":null}`,
		`{}`,
		`null`,
		`{"type":"oauth","refresh":"r","access":"a"}`, // missing expires
		`{"type":"oauth","refresh":"r","access":"a","expires":"soon"}`,
	}
	for _, input := range inputs {
		if _, err := ai.UnmarshalCredential([]byte(input)); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("UnmarshalCredential(%s) error = %v, want ErrCodec", input, err)
		}
	}
}

func TestOAuthCredentialUnmarshalRequiresOAuthDiscriminator(t *testing.T) {
	t.Parallel()

	var credential ai.OAuthCredential
	if err := json.Unmarshal([]byte(`{"type":"api_key","refresh":"r","access":"a","expires":1}`), &credential); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("OAuthCredential.UnmarshalJSON(wrong discriminator) error = %v, want ErrCodec", err)
	}
}

func TestInMemoryCredentialStoreReadListModifyDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()

	if got, err := store.Read(ctx, "anthropic", ai.AuthOperationOptions{}); err != nil || got != nil {
		t.Fatalf("Read(missing) = (%v, %v), want (nil, nil)", got, err)
	}

	stored := ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("sk-live")}
	post, err := store.Modify(ctx, "anthropic", func(_ context.Context, current ai.Credential) (ai.Credential, error) {
		if current != nil {
			t.Fatalf("Modify saw current = %#v, want nil for first write", current)
		}
		return stored, nil
	}, ai.AuthOperationOptions{})
	if err != nil || !reflect.DeepEqual(post, stored) {
		t.Fatalf("Modify(write) = (%#v, %v), want the stored credential", post, err)
	}

	// A modify that returns nil leaves the entry unchanged and returns the current.
	unchanged, err := store.Modify(ctx, "anthropic", func(_ context.Context, current ai.Credential) (ai.Credential, error) {
		return nil, nil
	}, ai.AuthOperationOptions{})
	if err != nil || !reflect.DeepEqual(unchanged, stored) {
		t.Fatalf("Modify(no-op) = (%#v, %v), want the current credential unchanged", unchanged, err)
	}

	// A second provider proves List enumerates every entry, sorted by id.
	if _, err := store.Modify(ctx, "openai", func(_ context.Context, _ ai.Credential) (ai.Credential, error) {
		return ai.OAuthCredential{OAuthCredentials: ai.OAuthCredentials{Refresh: "r", Access: "a", Expires: 1}, Type: ai.AuthTypeOAuth}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("Modify(openai) error = %v", err)
	}
	infos, err := store.List(ctx, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []ai.CredentialInfo{
		{ProviderID: "anthropic", Type: ai.AuthTypeAPIKey},
		{ProviderID: "openai", Type: ai.AuthTypeOAuth},
	}
	if !reflect.DeepEqual(infos, want) {
		t.Fatalf("List() = %#v, want sorted %#v", infos, want)
	}

	if err := store.Delete(ctx, "anthropic", ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got, err := store.Read(ctx, "anthropic", ai.AuthOperationOptions{}); err != nil || got != nil {
		t.Fatalf("Read(after delete) = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestInMemoryCredentialStoreModifyRejectsNilFunc(t *testing.T) {
	t.Parallel()

	store := ai.NewInMemoryCredentialStore()
	if _, err := store.Modify(context.Background(), "anthropic", nil, ai.AuthOperationOptions{}); err == nil {
		t.Fatal("Modify(nil fn) error = nil, want a rejection")
	}
}

func TestInMemoryCredentialStoreSerializesConcurrentModify(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()

	// Each writer reads the current counter, increments it, and writes it back.
	// A lost update would only happen if two read-modify-writes interleaved, so a
	// final total below writers proves the store failed to serialize.
	const writers = 32
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			_, _ = store.Modify(ctx, "anthropic", func(_ context.Context, current ai.Credential) (ai.Credential, error) {
				count := int64(0)
				if oauth, ok := current.(ai.OAuthCredential); ok {
					count = oauth.Expires
				}
				return ai.OAuthCredential{
					OAuthCredentials: ai.OAuthCredentials{Refresh: "r", Access: "a", Expires: count + 1},
					Type:             ai.AuthTypeOAuth,
				}, nil
			}, ai.AuthOperationOptions{})
		}()
	}
	wg.Wait()

	final, err := store.Read(ctx, "anthropic", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got := final.(ai.OAuthCredential).Expires; got != writers {
		t.Fatalf("serialized increment total = %d, want %d", got, writers)
	}
}

func TestInMemoryCredentialStoreHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	store := ai.NewInMemoryCredentialStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Read(ctx, "anthropic", ai.AuthOperationOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read(cancelled) error = %v, want context.Canceled", err)
	}
	if _, err := store.List(ctx, ai.AuthOperationOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("List(cancelled) error = %v, want context.Canceled", err)
	}
	if _, err := store.Modify(ctx, "anthropic", func(context.Context, ai.Credential) (ai.Credential, error) {
		t.Fatal("Modify fn ran under a cancelled context")
		return nil, nil
	}, ai.AuthOperationOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Modify(cancelled) error = %v, want context.Canceled", err)
	}
	if err := store.Delete(ctx, "anthropic", ai.AuthOperationOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete(cancelled) error = %v, want context.Canceled", err)
	}
}
