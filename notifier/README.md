# notifier

Notifier delivers Runtime Radar alerts through the configured notification channels: email, syslog, webhooks and AI
integrations. It also hosts the product's built-in chat assistant.

## Chat assistant

`AssistantController.Chat` answers one user turn and streams the answer back, so the interface can show the text as it
is written and name the tool the assistant is running while it works. grpc-gateway serves it as newline-delimited JSON
on `POST /api/v1/assistant/chat`.

The loop lives in `pkg/assistant`: at most 8 model turns, 120 seconds, 4 chats at a time. The tools come from
MCP Server over the Model Context Protocol, and the caller's own `Authorization` header travels with every one of
them, so RBAC and audit downstream name the person asking rather than this service.

No conversation is stored. The client replays the history it wants the model to see on every request, bounded here at
50 messages, 16 KB per message and 64 KB in total. A request over any of those bounds is refused rather than trimmed,
so the interface keeps the files a user attaches inside a budget of its own — see `ASSISTANT_MAX_ATTACHMENTS_BYTES` in
`radar-ui/libs/domains/assistant`, which leaves room in the message for the question itself.

### Modes

`mode` selects the instructions the assistant is given for the turn. It never widens what the assistant may do — the
same tools, the same limits, the same approval rule apply to all four.

| Mode | Used by | What it asks for |
| --- | --- | --- |
| `chat` (default) | the chat widget | an ordinary answer |
| `explain` | "Explain event" on an event page | what happened, how dangerous it is, ordinary explanations, what to check |
| `digest` | "Summary" on the events page | what the last period holds, what needs attention |
| `support` | "Report a problem" | an interview, ending in a request the interface turns into an email |

A `support` answer ends with a fenced block marked `support-request`, whose first line is `Subject:`. That is the
contract between the prompt and the interface, which reads the block back out to prefill the mail client.

### Tools that change the product

Most MCP tools only read. A few — `create_rule`, `delete_rule`, `create_api_token`, `delete_api_token` — change
Runtime Radar, and the loop never runs one on the model's word alone:

1. The model asks for a tool whose `readOnlyHint` is false.
2. The loop stops the turn, remembers the exact call, and streams a `confirmation` chunk: the tool, its title, the
   arguments verbatim, and an identifier. `stop_reason` is `confirmation_required`.
3. The user reads what would happen and presses the button.
4. The client sends the next request with `confirm_id`. The loop runs that stored call — not a call the model
   composes again — tells the model what came of it, and lets it report back.

The stored call expires after 10 minutes, can be used once, and only by the session it was issued to (the credential
is fingerprinted, never kept). A tool the MCP server says nothing about is treated as one that writes: being wrong in
that direction only costs a question.

`create_api_token` returns a credential. It arrives in a `secret` field of the tool result, which the loop lifts out
before the result reaches the model: the value is streamed to the user in a `secret` chunk and never becomes part of
a prompt sent to an LLM provider. The convention is documented in `mcp-server/README.md`.

`notifier_assistant_confirmed_calls_total{tool,status}` counts what was actually run this way — the series to alert
on, separate from the read-only tool traffic in `notifier_assistant_tool_calls_total`.

### Configuration

| Variable | Flag | Meaning |
| --- | --- | --- |
| `MCP_SERVER_URL` | `-mcpServerURL` | Streamable HTTP endpoint of MCP Server, for example `https://mcp-server:9000/mcp`. |

The assistant is offered in the interface only when an AI integration exists; without `MCP_SERVER_URL` the loop has no
tools and answers say so.
