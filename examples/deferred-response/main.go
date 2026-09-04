package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nankedr/pig/ai"
)

func main() {
	ctx := context.Background()
	pending := 2
	faux, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{Deferred: &ai.FauxDeferredOptions{PendingFetches: &pending}})
	if err != nil {
		log.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("ready"))
	if err != nil {
		log.Fatal(err)
	}
	faux.SetResponses([]ai.FauxResponseStep{response, response})
	model, _ := faux.GetModel()
	models := ai.CreateModels()
	models.SetProvider(faux.Provider)
	options := ai.ModelsSimpleStreamOptions{SimpleStreamOptions: ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}}
	submit := func() ai.DeferredHandle {
		message, err := models.CompleteSimple(ctx, model, ai.Context{}, options)
		if err != nil {
			log.Fatal(err)
		}
		handle, ok := message.Deferred.Value()
		if message.StopReason != ai.StopReasonDeferred || !ok {
			log.Fatalf("submission failed: %+v", message)
		}
		fmt.Printf("submitted: %s\n", message.StopReason)
		return handle
	}
	handle := submit()
	for {
		message, err := models.FetchDeferred(ctx, model, handle)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("poll: %s\n", message.StopReason)
		if message.StopReason == ai.StopReasonDeferred {
			continue
		}
		if message.StopReason != ai.StopReasonStop {
			log.Fatalf("fetch failed: %+v", message)
		}
		fmt.Println(message.Content[0].(ai.TextContent).Text)
		break
	}
	handle = submit()
	if err := models.CancelDeferred(ctx, model, handle); err != nil {
		log.Fatal(err)
	}
	message, err := models.FetchDeferred(ctx, model, handle)
	if err != nil {
		log.Fatal(err)
	}
	if message.StopReason != ai.StopReasonError {
		log.Fatalf("cancelled response unexpectedly succeeded: %+v", message)
	}
	fmt.Printf("after cancel: %s\n", message.StopReason)
}
