# AI MVP Plan

## Scope

- Implement MVP AI integration on top of the existing `notifier` service.
- Keep the diff minimal and follow the current project style.
- Do not create a new service and do not rename `notifier` in this task.
- First user scenario: explain a runtime event from the runtime event details page.

## Confirmed Decisions

- AI providers in MVP:
  - `OpenAI-compatible`
  - `Anthropic`
  - `Ollama`
- Users can manually specify any model name.
- Integration types must stay API-oriented, not brand-oriented.
- AI analysis result is ephemeral in the UI.
- Prompt for event explanation is fixed in backend for MVP.
- The action is available from `runtime event details`.
- The action opens a modal, then the user selects an AI integration and starts analysis manually.
- Show the action to all users who can access the page.
- Add i18n strings in the existing style.
- Add only minimal tests.

## Deferred

- No separate AI service.
- No renaming `notifier` to `integrator`.
- No async jobs, queues, or streaming.
- No automatic launch from rules.
- No persisted history of AI analysis results.
- No diagnostics flow where AI explores logs/popups/problems automatically.
- No custom headers in AI integration config for MVP unless they become necessary during implementation.

## Task 1: Backend in notifier

Implement MVP AI integration in the existing `notifier` service.

### Goals

- Add a new integration type: `ai`.
- Add storage and validation for AI integrations in the same style as existing integrations.
- Reuse existing secret encryption patterns.
- Add provider abstraction and support:
  - `OpenAI-compatible`
  - `Anthropic`
  - `Ollama`
- Add connection test for AI integrations.
- Add ephemeral runtime event analysis in "Explain event" mode.

### Expected API shape

- CRUD for `ai` integrations through the existing integration flow.
- Connection test endpoint for AI integrations.
- Analysis endpoint that accepts runtime event context and returns:
  - `summary`
  - `risk`
  - `possible_cause`
  - `next_steps`
  - `raw_text`

### Constraints

- Keep changes localized.
- Do not break existing integration types (`email`, `syslog`, `webhook`).
- Do not add persistence for analysis results.
- Do not implement the future diagnostics workflow in this task.

## Task 2: Infra and API wiring

Wire the new AI MVP into the existing routing and API structure.

### Goals

- Add the required routes in `reverse-proxy`.
- Update contracts and client bindings only where needed for AI MVP.
- Preserve the existing path-based routing style.

### Constraints

- Do not reorganize routing outside the minimum required changes.
- Do not touch unrelated services and routes.

## Task 3: Frontend in radar-ui

Implement the AI MVP in the existing frontend style.

### Goals

- Add a new integration type `ai` to the integrations UI.
- Add create/edit UI for AI integrations.
- Support provider types:
  - `OpenAI-compatible`
  - `Anthropic`
  - `Ollama`
- Keep `model` as a free-text field.
- Add an explicit "Test connection" button.
- Add an "Explain event" action to runtime event details.
- Open a modal where the user:
  - selects an AI integration
  - starts analysis manually
  - sees the ephemeral result
- Add i18n strings in the current style.

### Constraints

- Reuse existing modal/form/request patterns.
- Do not add a separate AI page.
- Do not persist analysis results.
- Do not implement diagnostics over logs/popups in this MVP.

## Implementation Order

1. Backend `notifier` support for `ai` integrations and providers.
2. Route and API wiring.
3. Frontend integration management UI.
4. Runtime event details modal and analysis flow.
5. Minimal tests and verification.

## Next Phase Ideas

- Rename `notifier` to a more generic integration-oriented service.
- Add persisted diagnostics history.
- Add diagnostics flow for product problems/popups with log access.
- Add richer provider configuration when needed.
- Add model presets and provider-specific UX improvements.
