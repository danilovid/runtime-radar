# Runtime Radar — контекст для Claude Code

Микросервисная система runtime-безопасности контейнеров (K8s): Tetragon (eBPF) →
runtime-monitor → RabbitMQ (queue runtime_events) → event-processor (цепочка WASM-детекторов,
TinyGo) → policy-enforcer (правила) / kube-manager (kill pod) / notifier (email, syslog,
webhook, ai) → RabbitMQ (queue history_events) → history-api → ClickHouse. UI: radar-ui
(Angular 19 + Nx + Koobiq + transloco). Роутинг: reverse-proxy (Caddy, path-based).
Auth: auth-center (JWT). Мультикластер: cluster-manager + cs-manager.

## Структура
- КАЖДЫЙ сервис — отдельный Go-модуль со своим go.mod (go 1.25) и своим vendor/.
  После изменения зависимостей: `go mod tidy && go mod vendor` внутри сервиса.
- Общий код — в lib/ (отдельный модуль): lib/rabbit (PublishConsumer поверх RabbitMQ,
  реконнект), lib/logger, lib/config (LookupEnv*), lib/security (cipher, jwt), lib/server.
- Шаблон сервиса: cmd/<name>/main.go (+init.go), pkg/{config,model,database,server,service,
  metrics,build}, api/*.proto (gRPC + grpc-gateway, HTTP-аннотации), migrations/ (goose),
  .helm/ (чарт), Dockerfile, Taskfile.yml.

## Конвенции
- Логи: zerolog (log.Info().Msgf(...)); ошибки: fmt.Errorf("can't ...: %w", err).
- Конфиг: только env + flags через lib/config, см. pkg/config/config.go любого сервиса.
- gRPC-сервисы: слои *_generic.go / *_auth.go / *_logging.go (см. notifier/pkg/service).
- Метрики Prometheus: pkg/metrics по образцу существующих.
- БД: GORM; Postgres — конфиги/состояние, ClickHouse — события (history-api).
- Кодогенерация proto — через Taskfile (task <service>:generate или см. Taskfile сервиса).
- Линт: golangci-lint, конфиг .golangci.yml в корне.
- UI: Nx-монорепо radar-ui; домены в libs/domains/*, фичи в libs/features/*;
  стейт — стори по образцу libs/domains/integration/src/lib/stores/*;
  i18n — libs/i18n-resources/en-US/*.json (+ ru-RU если есть); компоненты Koobiq.

## AI-подсистема (после PR #1)
- AI — тип интеграции в notifier: notifier/pkg/model/ai.go (GORM, API-ключ шифруется
  lib/security/cipher), notifier/pkg/ai/client.go (провайдеры: openai-compatible,
  anthropic, ollama), RPC TestAI и ExplainRuntimeEvent в notifier/api/integration.proto.
- UI: форма AI-интеграции в libs/domains/integration, модалка «Explain event»
  в libs/features/runtime/.../explain-event-modal.

## Правила безопасности (обязательны для всего AI-кода)
- Данные событий (аргументы процессов, пути) — недоверенные, контролируются атакующим:
  в промптах отделять данные от инструкций, никогда не исполнять то, что пришло из событий.
- LLM-вывод — только рекомендация: никаких автоматических блокировок/kill pod по нему.
- Секреты (API-ключи, kubeconfig) не должны попадать в промпты и логи.
- Перед отправкой телеметрии внешним LLM-провайдерам — маскировать секреты в аргументах
  (Bearer/password/token/base64-блоки).
