package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "pig-credentials-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "private", "auth.json")
	store, err := codingagent.NewAuthStorage(path)
	if err != nil {
		return err
	}
	ctx := context.Background()
	_, err = store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("synthetic-example-key")}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		return err
	}
	reopened, err := codingagent.NewAuthStorage(path)
	if err != nil {
		return err
	}
	models := ai.BuiltinModels(ai.CreateModelsOptions{Credentials: reopened})
	model, ok := models.GetModel("deepseek", "deepseek-v4-flash")
	if !ok {
		return fmt.Errorf("example model missing")
	}
	options := ai.ModelsSimpleStreamOptions{}
	options.Fetch = func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
		if request.Headers["Authorization"] != "Bearer synthetic-example-key" {
			return ai.FetchResponse{}, fmt.Errorf("saved credential was not restored")
		}
		return ai.FetchResponse{Status: 200, Body: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"credential restored\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")}, nil
	}
	result, err := models.CompleteSimple(ctx, model, ai.Context{}, options)
	if err != nil {
		return err
	}
	if result.StopReason != ai.StopReasonStop {
		return fmt.Errorf("example request failed")
	}
	infos, err := reopened.List(ctx, ai.AuthOperationOptions{})
	if err != nil {
		return err
	}
	for _, info := range infos {
		fmt.Println(info.ProviderID, info.Type)
	}
	if err := reopened.Delete(ctx, "deepseek", ai.AuthOperationOptions{}); err != nil {
		return err
	}
	fmt.Println("credential restored, request completed, credential deleted")
	return nil
}
