package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/statepath"
)

// AuthStorage owns a canonical file credential store. Read resolves key references;
// callers must finish project trust evaluation before reading executable values.
type AuthStorage struct{ path string }

func ResolveAuthPath(path string) (string, error) { return statepath.AuthPath(path) }

func NewAuthStorage(path ...string) (*AuthStorage, error) {
	value := ""
	if len(path) > 0 {
		value = path[0]
	}
	resolved, err := ResolveAuthPath(value)
	if err != nil {
		return nil, errors.New("cannot resolve credential path")
	}
	return &AuthStorage{path: resolved}, nil
}

func (s *AuthStorage) withLock(ctx context.Context, write bool, fn func(map[string]json.RawMessage) (bool, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := credentialHostAvailable(); err != nil {
		return err
	}
	if !write {
		if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
			_, err = fn(map[string]json.RawMessage{})
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return errors.New("cannot create credential directory")
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return errors.New("cannot open credential store")
	}
	defer f.Close()
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := lockAuthFile(lockCtx, f); err != nil {
		return err
	}
	defer unlockAuthFile(f)
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return errors.New("cannot read credential store")
	}
	fields := map[string]json.RawMessage{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
			return errors.New("invalid credential store")
		}
	}
	changed, err := fn(fields)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return errors.New("cannot encode credential store")
	}
	if err := f.Chmod(0600); err != nil {
		return errors.New("cannot secure credential file")
	}
	if err := f.Truncate(0); err != nil {
		return errors.New("cannot write credential store")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return errors.New("cannot write credential store")
	}
	if _, err := f.Write(data); err != nil {
		return errors.New("cannot write credential store")
	}
	if err := f.Sync(); err != nil {
		return errors.New("cannot sync credential store")
	}
	return nil
}

func decodeStoredCredential(raw json.RawMessage) (ai.Credential, error) {
	if raw == nil {
		return nil, nil
	}
	value, err := ai.UnmarshalCredential(raw)
	if err != nil {
		return nil, errors.New("invalid stored credential")
	}
	if key, ok := value.(ai.APIKeyCredential); ok && key.Key.IsNull() {
		return nil, errors.New("invalid stored credential")
	}
	return value, nil
}

func (s *AuthStorage) readRaw(ctx context.Context, provider ai.ProviderID) (ai.Credential, error) {
	var value ai.Credential
	err := s.withLock(ctx, false, func(fields map[string]json.RawMessage) (bool, error) {
		var err error
		value, err = decodeStoredCredential(fields[string(provider)])
		return false, err
	})
	return value, err
}

func (s *AuthStorage) Read(ctx context.Context, provider ai.ProviderID, _ ai.AuthOperationOptions) (ai.Credential, error) {
	value, err := s.readRaw(ctx, provider)
	if err != nil {
		return nil, err
	}
	if credential, ok := value.(ai.APIKeyCredential); ok {
		key, present := credential.Key.Value()
		if present {
			key, err = resolveCredentialValue(ctx, key, credential.Env)
			if err != nil {
				return nil, err
			}
		}
		if !present || key == "" {
			return nil, errors.New("stored API key could not be resolved")
		}
		credential.Key = ai.Some(key)
		value = credential
	}
	return value, nil
}

func (s *AuthStorage) List(ctx context.Context, _ ai.AuthOperationOptions) ([]ai.CredentialInfo, error) {
	result := []ai.CredentialInfo{}
	err := s.withLock(ctx, false, func(fields map[string]json.RawMessage) (bool, error) {
		for provider, raw := range fields {
			value, err := decodeStoredCredential(raw)
			if err != nil {
				return false, err
			}
			result = append(result, ai.CredentialInfo{ProviderID: ai.ProviderID(provider), Type: value.CredentialType()})
		}
		return false, nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].ProviderID < result[j].ProviderID })
	return result, err
}

func (s *AuthStorage) Modify(ctx context.Context, provider ai.ProviderID, fn ai.CredentialModifyFunc, _ ai.AuthOperationOptions) (ai.Credential, error) {
	if fn == nil {
		return nil, errors.New("credential modify function must not be nil")
	}
	var result ai.Credential
	err := s.withLock(ctx, true, func(fields map[string]json.RawMessage) (bool, error) {
		current, err := decodeStoredCredential(fields[string(provider)])
		if err != nil {
			return false, err
		}
		next, err := fn(ctx, current)
		if err != nil {
			return false, err
		}
		result = current
		if next == nil {
			return false, nil
		}
		data, err := ai.MarshalCredential(next)
		if err != nil {
			return false, errors.New("invalid replacement credential")
		}
		fields[string(provider)] = data
		result = next
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *AuthStorage) Delete(ctx context.Context, provider ai.ProviderID, _ ai.AuthOperationOptions) error {
	return s.withLock(ctx, true, func(fields map[string]json.RawMessage) (bool, error) {
		delete(fields, string(provider))
		return true, nil
	})
}

type headlessCredentials struct {
	ai.CredentialStore
	apiKey   *string
	provider ai.ProviderID
}

func (s headlessCredentials) Read(ctx context.Context, provider ai.ProviderID, options ai.AuthOperationOptions) (ai.Credential, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.apiKey != nil && (s.provider == "" || s.provider == provider) {
		if *s.apiKey == "" {
			return nil, errors.New("explicit API key must not be empty")
		}
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some(*s.apiKey)}, nil
	}
	credential, err := s.CredentialStore.Read(ctx, provider, options)
	if err != nil {
		return nil, err
	}
	if credential != nil && credential.CredentialType() == ai.AuthTypeOAuth {
		return nil, notImplemented("credential.oauth")
	}
	if value, ok := credential.(ai.APIKeyCredential); ok && provider == ai.ProviderIDDeepSeek {
		// The store already used scoped env to resolve the key; DeepSeek takes only the key.
		value.Env = nil
		credential = value
	}
	return credential, nil
}
