package ai

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// maxStreamLineBytes bounds one line of a streamed response. Deltas are small;
// this only has to be large enough that a long tool-call fragment doesn't
// truncate the answer.
const maxStreamLineBytes = 1024 * 1024

// sseDataPrefix and sseEventPrefix are the two fields of a server-sent event
// this code cares about.
const (
	sseDataPrefix  = "data:"
	sseEventPrefix = "event:"
	sseDone        = "[DONE]"
)

// readSSE walks a server-sent event stream, calling onEvent for every data
// payload. The event name is empty for providers that don't send one, which is
// the case for the OpenAI chat completions stream.
func readSSE(body io.Reader, onEvent func(event string, data []byte) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamLineBytes)

	event := ""

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		switch {
		case line == "":
			// A blank line ends an event; the next one starts unnamed.
			event = ""

		case strings.HasPrefix(line, sseEventPrefix):
			event = strings.TrimSpace(strings.TrimPrefix(line, sseEventPrefix))

		case strings.HasPrefix(line, sseDataPrefix):
			data := strings.TrimSpace(strings.TrimPrefix(line, sseDataPrefix))
			if data == "" || data == sseDone {
				continue
			}

			if err := onEvent(event, []byte(data)); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}

// readJSONLines walks a stream of newline-delimited JSON objects, which is how
// Ollama streams its answer.
func readJSONLines(body io.Reader, onLine func(data []byte) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamLineBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := onLine([]byte(line)); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// toolCallAccumulator assembles tool calls that arrive in fragments. OpenAI
// streams a call as an index plus pieces of its name and arguments, so nothing
// is usable until the stream ends.
type toolCallAccumulator struct {
	order []int
	calls map[int]*ToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{calls: make(map[int]*ToolCall)}
}

// add merges a fragment into the call at index. Empty values are ignored, so a
// fragment that only carries arguments doesn't erase the name.
func (a *toolCallAccumulator) add(index int, id, name, arguments string) {
	call, ok := a.calls[index]
	if !ok {
		call = &ToolCall{}
		a.calls[index] = call
		a.order = append(a.order, index)
	}

	if id != "" {
		call.ID = id
	}
	if name != "" {
		call.Name = name
	}
	if arguments != "" {
		call.Arguments = append(call.Arguments, arguments...)
	}
}

// result returns the assembled calls in the order they first appeared.
func (a *toolCallAccumulator) result() []ToolCall {
	if len(a.order) == 0 {
		return nil
	}

	calls := make([]ToolCall, 0, len(a.order))
	for _, index := range a.order {
		call := a.calls[index]
		if call.Name == "" {
			continue
		}

		if call.ID == "" {
			call.ID = toolCallID(index, call.Name)
		}

		call.Arguments = unquoteArguments(json.RawMessage(call.Arguments))
		calls = append(calls, *call)
	}

	return calls
}
