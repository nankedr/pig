package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nankedr/pig/ai"
)

func ResolveOffline(explicit bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PIG_OFFLINE"))) {
	case "1", "true", "yes":
		return true
	}
	return explicit
}

func newModelRuntime(ctx context.Context, options CreateModelRuntimeOptions, environment ai.ProviderEnv) (*ModelRuntime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("NewModelRuntime context must not be nil")
	}
	if (options.ModelsPath.IsSet() && !options.ModelsPath.IsNull()) || options.ModelsStore != nil || options.ModelsStorePath != "" || options.CatalogBaseURL != "" || (options.AllowModelNetwork && !ResolveOffline(options.Offline)) {
		return nil, notImplemented("NewModelRuntime")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	credentials := options.Credentials
	if credentials == nil {
		var err error
		credentials, err = NewAuthStorage(options.AuthPath)
		if err != nil {
			return nil, err
		}
	}
	config := ai.CreateModelsOptions{Credentials: headlessCredentials{CredentialStore: credentials}}
	if environment != nil {
		config.AuthContext = headlessAuthContext(environment)
	}
	r := &ModelRuntime{models: ai.BuiltinModels(config), credentials: credentials, auth: map[string]AuthStatus{}}
	if err := installCatalogSnapshot(r.models); err != nil {
		return nil, err
	}
	if options.RefreshOnCreate == nil || *options.RefreshOnCreate {
		_, err := r.GetAvailable(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		_ = err
	}
	return r, nil
}

func (r *ModelRuntime) refreshAvailability(ctx context.Context, providers []string) ([]ai.Model, error) {
	if ctx == nil {
		return nil, fmt.Errorf("availability context must not be nil")
	}
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	selected := r.models.GetProviders()
	if len(providers) > 0 {
		p, ok := r.models.GetProvider(ai.ProviderID(providers[0]))
		selected = nil
		if ok {
			selected = []ai.Provider{p}
		}
	}
	infos, err := r.credentials.List(ctx, ai.AuthOperationOptions{})
	if err != nil {
		return nil, err
	}
	stored := map[string]bool{}
	for _, info := range infos {
		stored[string(info.ProviderID)] = true
	}
	statuses := map[string]AuthStatus{}
	available := []ai.Model{}
	var failures []error
	for _, p := range selected {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := string(p.ID())
		statuses[id] = AuthStatus{}
		if len(p.GetModels()) == 0 {
			continue
		}
		check, err := r.models.CheckAuth(ctx, p.ID())
		if err != nil {
			if !errors.Is(err, ai.ErrNotImplemented) && !errors.Is(err, ErrNotImplemented) {
				failures = append(failures, err)
			}
			continue
		}
		auth, ok := check.Value()
		if !ok {
			continue
		}
		if auth.Type != ai.AuthTypeAPIKey {
			continue
		}
		status := AuthStatus{Configured: true, Source: "environment"}
		status.Label, _ = auth.Source.Value()
		if stored[id] {
			status.Source = "stored"
			status.Label = ""
		}
		models, err := r.models.GetAvailable(ctx, p.ID())
		if err != nil {
			failures = append(failures, err)
			continue
		}
		statuses[id] = status
		available = append(available, models...)
	}
	err = errors.Join(failures...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(providers) == 0 {
		r.auth = statuses
		r.available = cloneRuntimeModels(available)
	} else {
		for id, status := range statuses {
			r.auth[id] = status
		}
		kept := []ai.Model{}
		for _, m := range r.available {
			if string(m.Provider) != providers[0] {
				kept = append(kept, m)
			}
		}
		r.available = append(kept, cloneRuntimeModels(available)...)
	}
	r.availabilityError = ""
	if err != nil {
		r.availabilityError = "Availability refresh: " + err.Error()
	}
	return available, err
}

func cloneRuntimeModels(models []ai.Model) []ai.Model {
	result := make([]ai.Model, len(models))
	for i, m := range models {
		data, _ := json.Marshal(m)
		_ = json.Unmarshal(data, &result[i])
	}
	return result
}

func runtimeAuthOverrides(values []ModelRuntimeAuthOverrides) ai.AuthResolutionOverrides {
	if len(values) == 0 {
		return ai.AuthResolutionOverrides{}
	}
	v := values[0]
	r := ai.AuthResolutionOverrides{Env: v.Env, MinOAuthValidity: ai.Some(time.Duration(v.MinOAuthValidityMS) * time.Millisecond)}
	if v.APIKey != "" {
		r.APIKey = ai.Some(v.APIKey)
	}
	return r
}

func checkRuntimeAdapter(model ai.Model) error {
	if model.Provider != ai.ProviderIDDeepSeek || model.API != ai.APIOpenAICompletions {
		return notImplemented("ModelRuntime.Adapter." + string(model.API))
	}
	return nil
}
