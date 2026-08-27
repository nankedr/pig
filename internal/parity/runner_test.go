package parity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nankedr/pig/internal/parity"
)

func TestRunCaseComparesObservationsFromUnifiedDrivers(t *testing.T) {
	input := json.RawMessage(`{"arguments":["auth","--help"]}`)
	driver := func() parity.Driver {
		return parity.DriverFunc{
			SurfaceName: parity.SurfaceCLI,
			ObserveFunc: func(_ context.Context, got parity.Case) (parity.Observation, error) {
				if string(got.Input) != string(input) {
					t.Fatalf("driver input = %s, want %s", got.Input, input)
				}
				return parity.Observation{
					Stdout:     pointer("Usage: pig auth --help\n"),
					ExitStatus: &parity.ExitStatus{Code: pointer(0)},
				}, nil
			},
		}
	}

	result, err := parity.RunCase(context.Background(), parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/pig/auth-help",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         input,
		Observe:       []parity.Channel{parity.ChannelStdout, parity.ChannelExitStatus},
	}, driver(), driver())
	if err != nil {
		t.Fatalf("RunCase() = %v", err)
	}
	if !result.Match {
		t.Fatal("RunCase() did not report a match")
	}
	if result.CaseID != "cli/pig/auth-help" || result.CatalogID != "contract:cli/pig/auth-help" {
		t.Fatalf("result identity = %q / %q", result.CaseID, result.CatalogID)
	}
}

func TestObservationEncodesEveryAcceptanceSurface(t *testing.T) {
	events := []json.RawMessage{json.RawMessage(`{"type":"message_update"}`)}
	outcome := json.RawMessage(`{"stop_reason":"stop"}`)
	wire := []parity.WireObservation{{
		Direction: "outbound",
		Encoding:  "cbor",
		Data:      pointer([]byte{1, 2}),
		Value:     json.RawMessage(`{"type":"hello"}`),
	}}
	sessions := []parity.SessionState{{
		ID:     "session-1",
		Format: "jsonl-v3",
		Value:  json.RawMessage(`{"entries":1}`),
	}}
	files := []parity.FileState{{
		Path:     "session.jsonl",
		Exists:   true,
		Mode:     pointer(uint32(0o600)),
		Contents: pointer([]byte("session\n")),
	}}
	sideEffects := []parity.SideEffect{{
		Kind:   "file_write",
		Target: "session.jsonl",
		Detail: json.RawMessage(`{"bytes":8}`),
	}}
	observation := parity.Observation{
		Events:  &events,
		Outcome: outcome,
		Error: &parity.ErrorObservation{
			Type:    "provider_error",
			Code:    "rate_limit",
			Message: "retry later",
			Details: json.RawMessage(`{"status":429}`),
		},
		Stdout: pointer("answer\n"),
		Stderr: pointer("warning\n"),
		ExitStatus: &parity.ExitStatus{
			Code:   pointer(130),
			Signal: "interrupt",
		},
		Wire:        &wire,
		Sessions:    &sessions,
		Files:       &files,
		SideEffects: &sideEffects,
	}

	got, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"events":[{"type":"message_update"}],"outcome":{"stop_reason":"stop"},"error":{"type":"provider_error","code":"rate_limit","message":"retry later","details":{"status":429}},"stdout":"answer\n","stderr":"warning\n","exit_status":{"code":130,"signal":"interrupt"},"wire":[{"direction":"outbound","encoding":"cbor","data":"AQI=","value":{"type":"hello"}}],"sessions":[{"id":"session-1","format":"jsonl-v3","value":{"entries":1}}],"files":[{"path":"session.jsonl","exists":true,"mode":384,"contents":"c2Vzc2lvbgo="}],"side_effects":[{"kind":"file_write","target":"session.jsonl","detail":{"bytes":8}}]}`
	if string(got) != want {
		t.Fatalf("observation JSON = %s, want %s", got, want)
	}
}

func TestObservationDistinguishesUnobservedFromObservedEmpty(t *testing.T) {
	emptyEvents := []json.RawMessage{}
	emptyWire := []parity.WireObservation{}
	emptySessions := []parity.SessionState{}
	emptyFiles := []parity.FileState{}
	emptyEffects := []parity.SideEffect{}
	observed := parity.Observation{
		Events:      &emptyEvents,
		Stdout:      pointer(""),
		Stderr:      pointer(""),
		ExitStatus:  &parity.ExitStatus{Code: pointer(0)},
		Wire:        &emptyWire,
		Sessions:    &emptySessions,
		Files:       &emptyFiles,
		SideEffects: &emptyEffects,
	}

	unobservedJSON, err := json.Marshal(parity.Observation{})
	if err != nil {
		t.Fatal(err)
	}
	observedJSON, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	if string(unobservedJSON) != `{}` {
		t.Fatalf("unobserved JSON = %s", unobservedJSON)
	}
	want := `{"events":[],"stdout":"","stderr":"","exit_status":{"code":0},"wire":[],"sessions":[],"files":[],"side_effects":[]}`
	if string(observedJSON) != want {
		t.Fatalf("observed-empty JSON = %s, want %s", observedJSON, want)
	}
}

func TestDriverFuncIsTheUnifiedBoundaryForEverySurface(t *testing.T) {
	for _, surface := range []parity.Surface{
		parity.SurfaceCLI,
		parity.SurfaceGoSDK,
		parity.SurfacePersistenceProtocol,
		parity.SurfaceTUIPlatform,
	} {
		t.Run(string(surface), func(t *testing.T) {
			outcome := json.RawMessage(`{"ok":true}`)
			driver := parity.DriverFunc{
				SurfaceName: surface,
				ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
					return parity.Observation{Outcome: outcome}, nil
				},
			}
			result, err := parity.RunCase(context.Background(), parity.Case{
				SchemaVersion: parity.CaseSchemaVersion,
				ID:            "surface/" + string(surface),
				CatalogID:     "catalog/" + string(surface),
				Surface:       surface,
				Input:         json.RawMessage(`{}`),
				Observe:       []parity.Channel{parity.ChannelOutcome},
			}, driver, driver)
			if err != nil || !result.Match {
				t.Fatalf("RunCase(%s) = match %v, error %v", surface, result.Match, err)
			}
		})
	}
}

func TestRunCaseRejectsDriverFromAnotherSurface(t *testing.T) {
	called := false
	driver := func(surface parity.Surface) parity.Driver {
		return parity.DriverFunc{
			SurfaceName: surface,
			ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
				called = true
				return parity.Observation{}, nil
			},
		}
	}

	_, err := parity.RunCase(context.Background(), parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "sdk/wrong-driver",
		CatalogID:     "contract:sdk/example",
		Surface:       parity.SurfaceGoSDK,
		Input:         json.RawMessage(`{}`),
		Observe:       []parity.Channel{parity.ChannelOutcome},
	}, driver(parity.SurfaceCLI), driver(parity.SurfaceGoSDK))
	if !errors.Is(err, parity.ErrSurfaceMismatch) {
		t.Fatalf("RunCase() error = %v, want ErrSurfaceMismatch", err)
	}
	if called {
		t.Fatal("a driver ran before surface validation")
	}
}

func TestRunCaseRejectsInvalidDeclarationsBeforeDriversRun(t *testing.T) {
	valid := parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/pig/auth-help",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         json.RawMessage(`{"arguments":["auth","--help"]}`),
		Observe:       []parity.Channel{parity.ChannelStdout},
	}
	tests := map[string]func(*parity.Case){
		"schema":     func(c *parity.Case) { c.SchemaVersion = "" },
		"id":         func(c *parity.Case) { c.ID = "" },
		"catalog id": func(c *parity.Case) { c.CatalogID = "" },
		"surface":    func(c *parity.Case) { c.Surface = "browser" },
		"input":      func(c *parity.Case) { c.Input = nil },
		"input json": func(c *parity.Case) { c.Input = json.RawMessage(`{`) },
		"observe":    func(c *parity.Case) { c.Observe = nil },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			declaration := valid
			mutate(&declaration)
			called := false
			driver := parity.DriverFunc{
				SurfaceName: parity.SurfaceCLI,
				ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
					called = true
					return parity.Observation{}, nil
				},
			}
			_, err := parity.RunCase(context.Background(), declaration, driver, driver)
			if !errors.Is(err, parity.ErrInvalidCase) {
				t.Fatalf("RunCase() error = %v, want ErrInvalidCase", err)
			}
			if called {
				t.Fatal("driver ran before case validation")
			}
		})
	}
}

func TestRunCaseRejectsMissingDeclaredObservation(t *testing.T) {
	driver := parity.DriverFunc{
		SurfaceName: parity.SurfaceCLI,
		ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
			return parity.Observation{}, nil
		},
	}
	_, err := parity.RunCase(context.Background(), parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/missing-output",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         json.RawMessage(`{}`),
		Observe:       []parity.Channel{parity.ChannelStdout},
	}, driver, driver)
	if !errors.Is(err, parity.ErrIncompleteObservation) {
		t.Fatalf("RunCase() error = %v, want ErrIncompleteObservation", err)
	}
}

func TestRunCaseFailsOnAnUndeclaredDifferenceAndKeepsRawObservations(t *testing.T) {
	driver := func(stdout string) parity.Driver {
		return parity.DriverFunc{
			SurfaceName: parity.SurfaceCLI,
			ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
				return parity.Observation{Stdout: pointer(stdout)}, nil
			},
		}
	}
	result, err := parity.RunCase(context.Background(), parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/pig/auth-help",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         json.RawMessage(`{"arguments":["auth","--help"]}`),
		Observe:       []parity.Channel{parity.ChannelStdout},
	}, driver("pi auth\n"), driver("pig auth\n"))
	if !errors.Is(err, parity.ErrMismatch) {
		t.Fatalf("RunCase() error = %v, want ErrMismatch", err)
	}
	if result.Match {
		t.Fatal("RunCase() reported a match")
	}
	if len(result.Differences) != 1 || result.Differences[0].Path != "stdout" {
		t.Fatalf("differences = %+v, want stdout", result.Differences)
	}
	if result.Oracle.Stdout == nil || *result.Oracle.Stdout != "pi auth\n" || result.Pig.Stdout == nil || *result.Pig.Stdout != "pig auth\n" {
		t.Fatalf("raw observations changed: oracle=%+v pig=%+v", result.Oracle, result.Pig)
	}
}

func TestRunCaseComparesJSONChannelsSemantically(t *testing.T) {
	observation := func(reverse bool) parity.Observation {
		object := json.RawMessage(`{"a":1,"b":2,"big":9007199254740993,"zero":0}`)
		if reverse {
			object = json.RawMessage(`{ "zero": -0, "big": 9007199254740993.0, "b": 2.0, "a": 1e0 }`)
		}
		events := []json.RawMessage{object}
		wire := []parity.WireObservation{{Direction: "inbound", Encoding: "json", Data: pointer([]byte("same")), Value: object}}
		sessions := []parity.SessionState{{ID: "session", Value: object}}
		effects := []parity.SideEffect{{Kind: "record", Detail: object}}
		return parity.Observation{
			Events:      &events,
			Outcome:     object,
			Error:       &parity.ErrorObservation{Message: "failed", Details: object},
			Wire:        &wire,
			Sessions:    &sessions,
			SideEffects: &effects,
		}
	}
	driver := func(value parity.Observation) parity.Driver {
		return parity.DriverFunc{
			SurfaceName: parity.SurfaceGoSDK,
			ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
				return value, nil
			},
		}
	}

	result, err := parity.RunCase(context.Background(), parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "sdk/semantic-json",
		CatalogID:     "contract:sdk/semantic-json",
		Surface:       parity.SurfaceGoSDK,
		Input:         json.RawMessage(`{}`),
		Observe: []parity.Channel{
			parity.ChannelEvents, parity.ChannelOutcome, parity.ChannelError,
			parity.ChannelWire, parity.ChannelSessions, parity.ChannelSideEffects,
		},
	}, driver(observation(false)), driver(observation(true)))
	if err != nil || !result.Match {
		t.Fatalf("RunCase() = match %v, differences %+v, error %v", result.Match, result.Differences, err)
	}
}

func TestBrandTokenNormalizationIsCaseLocalAndKeepsRawObservations(t *testing.T) {
	oracleStdout := "pi auth print-api-key\npi auth print-bearer-token\npi auth check\nexpired credentials stay expired\n"
	pigStdout := "pig auth print-api-key\npig auth print-bearer-token\npig auth check\nexpired credentials stay expired\n"
	driver := func(stdout string) parity.Driver {
		return parity.DriverFunc{
			SurfaceName: parity.SurfaceCLI,
			ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
				return parity.Observation{Stdout: pointer(stdout)}, nil
			},
		}
	}
	result, err := parity.RunCase(context.Background(), parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/pig/auth-help",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         json.RawMessage(`{"arguments":["auth","--help"]}`),
		Observe:       []parity.Channel{parity.ChannelStdout},
		Normalizations: []parity.Normalization{{
			Target:       "/stdout",
			Kind:         parity.NormalizationBrandToken,
			Oracle:       "pi",
			Pig:          "pig",
			ExactMatches: 3,
		}},
	}, driver(oracleStdout), driver(pigStdout))
	if err != nil || !result.Match {
		t.Fatalf("RunCase() = match %v, differences %+v, error %v", result.Match, result.Differences, err)
	}
	if result.Oracle.Stdout == nil || *result.Oracle.Stdout != oracleStdout {
		t.Fatalf("raw oracle stdout = %v", result.Oracle.Stdout)
	}
	if result.NormalizedOracle.Stdout == nil || *result.NormalizedOracle.Stdout != pigStdout {
		t.Fatalf("normalized oracle stdout = %v, want %q", result.NormalizedOracle.Stdout, pigStdout)
	}
	if result.NormalizedPig.Stdout == nil || *result.NormalizedPig.Stdout != pigStdout {
		t.Fatalf("normalized Pig stdout = %v, want %q", result.NormalizedPig.Stdout, pigStdout)
	}
	if len(result.Normalizations) != 1 || result.Normalizations[0].Matches != 3 {
		t.Fatalf("normalization applications = %+v", result.Normalizations)
	}
}

func TestNormalizationRejectsBroadDeclarationsBeforeDriversRun(t *testing.T) {
	for _, rule := range []parity.Normalization{
		{Target: "/", Kind: parity.NormalizationBrandToken, Oracle: "pi", Pig: "pig", ExactMatches: 3},
		{Target: "/stdout", Kind: parity.NormalizationKind("substring"), Oracle: "pi", Pig: "pig", ExactMatches: 3},
		{Target: "/stdout", Kind: parity.NormalizationBrandToken, Oracle: ".*", Pig: "pig", ExactMatches: 3},
		{Target: "/stdout", Kind: parity.NormalizationBrandToken, Oracle: "i", Pig: "pig", ExactMatches: 3},
		{Target: "/stdout", Kind: parity.NormalizationBrandToken, Oracle: "pi", Pig: "pi", ExactMatches: 3},
		{Target: "/stdout", Kind: parity.NormalizationBrandToken, Oracle: "failure", Pig: "success", ExactMatches: 1},
		{Target: "/stdout", Kind: parity.NormalizationBrandToken, Oracle: "pi", Pig: "pig", ExactMatches: 0},
	} {
		t.Run(string(rule.Kind)+"/"+rule.Target+"/"+rule.Oracle, func(t *testing.T) {
			called := false
			driver := parity.DriverFunc{
				SurfaceName: parity.SurfaceCLI,
				ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
					called = true
					return parity.Observation{}, nil
				},
			}
			_, err := parity.RunCase(context.Background(), parity.Case{
				SchemaVersion:  parity.CaseSchemaVersion,
				ID:             "cli/invalid-normalization",
				CatalogID:      "contract:cli/pig/auth-help",
				Surface:        parity.SurfaceCLI,
				Input:          json.RawMessage(`{}`),
				Observe:        []parity.Channel{parity.ChannelStdout},
				Normalizations: []parity.Normalization{rule},
			}, driver, driver)
			if !errors.Is(err, parity.ErrInvalidNormalization) {
				t.Fatalf("RunCase() error = %v, want ErrInvalidNormalization", err)
			}
			if called {
				t.Fatal("driver ran before normalization validation")
			}
		})
	}
}

func TestNormalizationRejectsDuplicateRules(t *testing.T) {
	rule := parity.Normalization{Target: "/stdout", Kind: parity.NormalizationBrandToken, Oracle: "pi", Pig: "pig", ExactMatches: 1}
	driver := parity.DriverFunc{SurfaceName: parity.SurfaceCLI, ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
		return parity.Observation{Stdout: pointer("pi")}, nil
	}}
	_, err := parity.RunCase(context.Background(), parity.Case{
		SchemaVersion:  parity.CaseSchemaVersion,
		ID:             "cli/duplicate-normalization",
		CatalogID:      "contract:cli/pig/auth-help",
		Surface:        parity.SurfaceCLI,
		Input:          json.RawMessage(`{}`),
		Observe:        []parity.Channel{parity.ChannelStdout},
		Normalizations: []parity.Normalization{rule, rule},
	}, driver, driver)
	if !errors.Is(err, parity.ErrInvalidNormalization) {
		t.Fatalf("RunCase() error = %v, want ErrInvalidNormalization", err)
	}
}

func TestCaseDeclarationHasAStableSerializableBoundary(t *testing.T) {
	declaration := parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/pig/auth-help",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         json.RawMessage(`{"arguments":["auth","--help"]}`),
		Observe:       []parity.Channel{parity.ChannelStdout, parity.ChannelStderr, parity.ChannelExitStatus},
	}
	got, err := json.Marshal(declaration)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":"1.0.0","id":"cli/pig/auth-help","catalog_id":"contract:cli/pig/auth-help","surface":"cli","input":{"arguments":["auth","--help"]},"observe":["stdout","stderr","exit_status"]}`
	if string(got) != want {
		t.Fatalf("case JSON = %s, want %s", got, want)
	}
}

func pointer[T any](value T) *T { return &value }
