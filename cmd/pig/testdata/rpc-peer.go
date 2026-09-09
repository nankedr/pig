//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

func main() {
	mode := os.Getenv("RPC_PEER_MODE")
	if mode == "startup-exit" {
		fmt.Fprintln(os.Stderr, "fixture startup failure")
		os.Exit(43)
	}
	if mode == "startup-hang" {
		time.Sleep(time.Minute)
		return
	}
	var mu sync.Mutex
	write := func(value any) {
		mu.Lock()
		defer mu.Unlock()
		data, _ := json.Marshal(value)
		os.Stdout.Write(append(data, '\n'))
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var command map[string]any
		json.Unmarshal(scanner.Bytes(), &command)
		if command["type"] == "get_state" {
			write(map[string]any{"id": command["id"], "type": "response", "command": "get_state", "success": true, "data": map[string]any{"isStreaming": true}})
			switch mode {
			case "stdin-closed":
				os.Stdin.Close()
				time.Sleep(time.Minute)
				return
			case "backpressure":
				time.Sleep(time.Minute)
				return
			}
			continue
		}
		if mode == "exit" {
			os.Exit(44)
		}
		if mode == "eof" {
			os.Stdout.Close()
			time.Sleep(time.Minute)
			return
		}
		if mode == "framing" {
			write(map[string]any{"id": command["id"], "type": "response", "command": "prompt", "success": true})
			wire := []byte("{\"type\":\"queue_update\",\"steering\":[\"你好😀\u2028\u2029\"],\"followUp\":[]}\r\n{\"type\":\"agent_settled\"}")
			for _, b := range wire {
				os.Stdout.Write([]byte{b})
			}
			return
		}
		if mode == "cancel" {
			if command["message"] == "slow" {
				go func(id any) {
					time.Sleep(200 * time.Millisecond)
					write(map[string]any{"id": id, "type": "response", "command": "prompt", "success": false, "error": "slow"})
				}(command["id"])
			} else {
				write(map[string]any{"id": command["id"], "type": "response", "command": "prompt", "success": false, "error": command["message"]})
			}
		}
	}
}
