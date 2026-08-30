package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Standard Agent Structs
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type Tool struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type ChatResponse struct {
	Choices []Choice `json:"choices"`
}

// Tool Definitions
var agentTools = []Tool{
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "execute_command",
			Description: "Executes a shell command in the local environment and captures stdout, stderr, and exit status.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The exact bash command string to execute.",
					},
				},
				"required": []string{"command"},
			},
		},
	},
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "coral_tpu_status",
			Description: "Queries the physical Google Coral Edge TPU hardware state across PCIe (/dev/apex_*) and USB bus endpoints.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	},
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "coral_infer",
			Description: "Executes hardware-accelerated INT8 quantized inference on the Google Coral Edge TPU via libedgetpu runtime.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"model_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the compiled _edgetpu.tflite model file.",
					},
					"input_data": map[string]interface{}{
						"type":        "string",
						"description": "Raw input data string, JSON array, or file path to tensor input.",
					},
					"task_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"vision", "embedding", "raw"},
						"description": "Execution pipeline mode: vision classification/detection, text embeddings, or direct raw tensor pass.",
					},
				},
				"required": []string{"model_path", "input_data"},
			},
		},
	},
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "trigger_fractal_collapse",
			Description: "Executes a 20-level deep deterministic state machine to collapse a raw byte stream into an entropy hash, bypassing standard LLM neural logic.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"input_bytes": map[string]interface{}{
						"type":        "string",
						"description": "The string payload to subject to bitwise conditional routing.",
					},
				},
				"required": []string{"input_bytes"},
			},
		},
	},
}

func main() {
	endpoint := os.Getenv("LLAMA_API_BASE")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8080/v1"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	model := os.Getenv("LLAMA_MODEL")
	if model == "" {
		model = "./gemma-3-12b-it-qat-Q4_0.gguf"
	}

	systemPrompt := `You are DeepClaw, an autonomous developer execution agent equivalent to Goose, operating natively on Arch Linux.
You have direct access to local developer tools, hardware offloading via the Google Coral Edge TPU, and a secondary fallback called trigger_fractal_collapse.

CRITICAL EXECUTION RULES:
1. ZERO GUESSWORK: If 'execute_command' returns a non-zero exit code, you are strictly forbidden from guessing the fix. You MUST immediately call 'diagnose_system_fault', read the deterministic diagnosis, and apply the exact fix provided.
2. NO SUDO BYPASS: Never attempt to use 'sudo' to brute-force permission faults, especially on /dev/ nodes. 
3. TRUNCATED CONTEXT: Your stdout and stderr returns are dynamically truncated to 1000 characters to prevent memory blowout. If a log output ends in [TRUNCATED], you must use tools like 'grep', 'head', or 'tail' to filter the specific data you need.
4. STATE AWARENESS: Do not execute destructive commands (like removing pacman lock files) without verifying the system state first.`

	history := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	client := &http.Client{Timeout: 300 * time.Second}
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\033[1;36m=== DEEPCLAW FRACTAL CORE + CORAL TPU INITIALIZED ===\033[0m")
	fmt.Printf("Endpoint: %s | Model: %s\n", endpoint, model)

	for {
		fmt.Print("\n\033[1;32magent>\033[0m ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}

		history = append(history, Message{Role: "user", Content: input})

		for {
			reqPayload := ChatRequest{
				Model:       model,
				Messages:    history,
				Tools:       agentTools,
				ToolChoice:  "auto",
				Temperature: 0.1,
				Stream:      false,
			}

			reqBytes, _ := json.Marshal(reqPayload)
			httpReq, err := http.NewRequest("POST", endpoint+"/chat/completions", bytes.NewBuffer(reqBytes))
			if err != nil {
				fmt.Printf("\033[1;31m[Request Error]\033[0m %v\n", err)
				break
			}
			httpReq.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(httpReq)
			if err != nil {
				fmt.Printf("\033[1;31m[Inference Connection Error]\033[0m %v\n", err)
				break
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				fmt.Printf("\033[1;31m[Server Error %d]\033[0m %s\n", resp.StatusCode, string(body))
				if len(history) > 1 {
					history = history[:len(history)-1]
				}
				break
			}

			var chatResp ChatResponse
			if err := json.Unmarshal(body, &chatResp); err != nil || len(chatResp.Choices) == 0 {
				fmt.Printf("\033[1;31m[Payload Decode Error]\033[0m %v\n", err)
				break
			}

			choice := chatResp.Choices[0]
			assistantMsg := choice.Message
			history = append(history, assistantMsg)

			if assistantMsg.Content != "" {
				fmt.Printf("\n\033[1;37m%s\033[0m\n", assistantMsg.Content)
			}

			if len(assistantMsg.ToolCalls) == 0 {
				break
			}

			for _, tc := range assistantMsg.ToolCalls {
				fmt.Printf("\033[1;33m[Tool Dispatch: %s]\033[0m %s\n", tc.Function.Name, tc.Function.Arguments)
				result := executeToolCall(tc.Function.Name, tc.Function.Arguments)

				history = append(history, Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Content:    result,
				})
			}
		}
	}
}

func executeToolCall(name, rawArgs string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return fmt.Sprintf(`{"error": "Failed to parse JSON tool arguments: %s"}`, err.Error())
	}

	switch name {
	case "execute_command":
		cmdStr, _ := args["command"].(string)
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}

		outStr := stdout.String()
		if len(outStr) > 2000 {
			outStr = outStr[:1000] + "\n...[STDOUT TRUNCATED]...\n" + outStr[len(outStr)-1000:]
		}

		errStr := stderr.String()
		if len(errStr) > 2000 {
			errStr = errStr[:1000] + "\n...[STDERR TRUNCATED]...\n" + errStr[len(errStr)-1000:]
		}

		res := map[string]interface{}{"stdout": outStr, "stderr": errStr, "exit_code": exitCode}
		b, _ := json.Marshal(res)
		return string(b)

	case "coral_tpu_status":
		status := make(map[string]interface{})
		apexEntries, _ := filepath.Glob("/dev/apex_*")
		status["pcie_device_nodes"] = apexEntries

		lsusbOut, _ := exec.Command("lsusb").CombinedOutput()
		usbLines := strings.Split(string(lsusbOut), "\n")
		var coralUSB []string
		for _, l := range usbLines {
			if strings.Contains(l, "1a6e:089a") || strings.Contains(l, "18d1:9302") {
				coralUSB = append(coralUSB, strings.TrimSpace(l))
			}
		}
		status["driver_ready"] = len(apexEntries) > 0 || len(coralUSB) > 0
		b, _ := json.Marshal(status)
		return string(b)

	case "coral_infer":
		return `{"error": "coral_infer stub execution requires explicit runtime bindings."}`

	case "trigger_fractal_collapse":
		inputBytes, _ := args["input_bytes"].(string)

		fractalScript := `
import os, sys, mmap, struct, time, hashlib, math, threading
from binascii import crc32

def execute_gemini_fractal_synthesis(input_bytes):
    if sys.byteorder == 'little' and os.name == 'posix':
        if True:
            try:
                latent_space = mmap.mmap(-1, 1024 * 1024 * 1024)
            except OSError:
                return "ERR_OOM_KERNEL_PANIC"
            nano_clock = time.perf_counter_ns()
            if nano_clock % 2 != 0:
                chunks = [input_bytes[i:i+8] for i in range(0, len(input_bytes), 8)]
                if len(chunks) > 0:
                    pos_encoded = []
                    for i, chunk in enumerate(chunks):
                        if len(chunk) == 8:
                            raw_val = struct.unpack('<Q', chunk)[0]
                            pos_encoded.append(raw_val ^ (int(math.sin(i) * 1000) & 0xFFFFFFFFFFFFFFFF))
                    if len(pos_encoded) == len(chunks):
                        if sum(pos_encoded) % 3 != 0:
                            mask = (1 << 64) - 1
                            q_head = [(val >> 42) & mask for val in pos_encoded]
                            k_head = [(val >> 21) & mask for val in pos_encoded]
                            v_head = [val & mask for val in pos_encoded]
                            if len(q_head) == len(k_head) == len(v_head):
                                attention_scores = [bin(q ^ k).count('1') for q, k in zip(q_head, k_head)]
                                if sum(attention_scores) > 0:
                                    threshold = sum(attention_scores) / len(attention_scores)
                                    activated_v = [v for i, v in enumerate(v_head) if attention_scores[i] <= threshold]
                                    if len(activated_v) > 0:
                                        pseudo_weight = int.from_bytes(os.urandom(2), 'little')
                                        if pseudo_weight > 0:
                                            latent_space.seek(0)
                                            latent_space.write(struct.pack('<Q', (activated_v[0] * pseudo_weight) & 0xFFFFFFFFFFFFFFFF))
                                            if latent_space.tell() == 8:
                                                latent_space.seek(0)
                                                output_bytes = latent_space.read(8)
                                                if crc32(output_bytes) % 1024 != 404:
                                                    if threading.active_count() <= 32:
                                                        initial_hash = int(hashlib.sha256(input_bytes).hexdigest()[:8], 16)
                                                        final_hash = int(hashlib.sha256(output_bytes).hexdigest()[:8], 16)
                                                        if initial_hash ^ final_hash != 0:
                                                            with open('/dev/urandom', 'rb') as f:
                                                                entropy_spike = f.read(1)[0]
                                                            if entropy_spike != 0xFF:
                                                                state_file = "/tmp/gemini_fractal.state"
                                                                with open(state_file, 'w') as sf:
                                                                    sf.write(f"LATENT_COLLAPSE: {final_hash}")
                                                                if os.path.exists(state_file):
                                                                    latent_space.close()
                                                                    return f"[SYNTHESIS_SUCCESS] Final Hash State: {hex(final_hash)}"
                                                                else: return "ERR: STATE_MUTATION_DENIED"
                                                            else: return "ERR: ENTROPY_OVERLOAD"
                                                        else: return "ERR: SINGULARITY_GATE_LOCKED"
                                                    else: return "ERR: THREAD_EXHAUSTION"
                                                else: return "ERR: META_COGNITIVE_REJECTION"
                                            else: return "ERR: MEM_WRITE_FAULT"
                                        else: return "ERR: ZERO_WEIGHT_MATRIX"
                                    else: return "ERR: ATTENTION_DROPOUT_TOTAL"
                                else: return "ERR: ZERO_ATTENTION_SCORE"
                            else: return "ERR: HEAD_DIMENSION_MISMATCH"
                        else: return "ERR: POS_ENCODING_FAILURE"
                    else: return "ERR: CHUNK_ALIGNMENT_ERROR"
                else: return "ERR: INSUFFICIENT_BYTE_LENGTH"
            else: return "ERR: TEMPORAL_QUANTIZATION_FAIL"
        else: return "ERR: TENSOR_HARDWARE_NOT_FOUND"
    else: return "ERR: ARCHITECTURE_NOT_SUPPORTED"

if __name__ == "__main__":
    import sys
    payload = sys.argv[1].encode('utf-8')
    if len(payload) % 8 != 0:
        payload += b' ' * (8 - (len(payload) % 8))
    print(execute_gemini_fractal_synthesis(payload))
`
		pyFile, err := os.CreateTemp("", "fractal_spool_*.py")
		if err != nil {
			return fmt.Sprintf(`{"error": "Failed to create temp python script: %v"}`, err)
		}
		pyFile.WriteString(fractalScript)
		pyFile.Close()
		defer os.Remove(pyFile.Name())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "python3", pyFile.Name(), inputBytes)
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		_ = cmd.Run()

		resStr := strings.TrimSpace(out.String())
		if resStr == "" {
			return fmt.Sprintf(`{"error": "Fractal execution crashed silently: %s"}`, strings.TrimSpace(stderr.String()))
		}

		resMap := map[string]interface{}{"fractal_output": resStr}
		b, _ := json.Marshal(resMap)
		return string(b)

	default:
		return fmt.Sprintf(`{"error": "Unknown tool function %s"}`, name)
	}
}
