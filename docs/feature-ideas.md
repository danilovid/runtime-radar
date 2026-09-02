# Runtime Radar — предложения по фичам

Черновик идей для продуктового roadmap. Каждая фича привязана к компонентам, уже существующим в репозитории.

## Часть 1

### 1. Per-integration настройки AI: таймаут, max_tokens, температура

Логичное продолжение текущего MVP (`ai-mvp-notifier`). Сейчас `defaultTimeout`, `defaultMaxTokens`, `temperature` захардкожены в `notifier/pkg/ai/client.go`.

Что сделать:
- Добавить в `model.AI` и `api.AI` поля `timeout_seconds`, `max_tokens`, `temperature`, `system_prompt_override`.
- В UI — секцию "Advanced" в `integration-ai-form`.
- Миграция БД: расширить таблицу интеграций.

Value: разные модели (быстрые/медленные, дешёвые/дорогие) требуют разного бюджета; сейчас приходится перекатывать notifier.

Усилие: S (1–2 дня).

---

### 2. AI-ассистент создания детекторов и правил

Пользователь описывает на естественном языке («детектор на чтение `/etc/shadow` в prod-namespace») — LLM генерирует готовую Tetragon-спеку + severity + шаблон правила ответа.

Что сделать:
- Новый RPC `/api/v1/integration/ai/suggest-detector` (рядом с `explain-runtime-event`).
- Few-shot промпт с примерами существующих детекторов из `policy-enforcer`.
- В UI — шаг «Сгенерировать с AI» при создании детектора/правила.

Value: снижает порог входа для новых пользователей, продающая фича для демо.

Усилие: M (3–5 дней).

---

### 3. Incident clustering + batch explain

Сейчас `explain` идёт по одному событию — дорого по токенам и шумно в UI. Группировать близкие события (один pod + один detector + один binary в окне 10 мин) в инциденты и вызывать AI один раз на группу.

Что сделать:
- Дополнить `event-processor`: scoring по `(pod, detector, binary)` + sliding window.
- Новая таблица `incidents` в ClickHouse с ссылками на исходные события.
- В UI — шапка «Похожих событий: 47» в детали события.

Value: кратно меньше токенов, понятнее UX.

Усилие: M–L (1–2 недели).

---

### 4. Chat-ops: Telegram/Slack integration с actions

Сейчас в `notifier` есть Email/Webhook/Syslog. Добавить Telegram и Slack как отдельные типы интеграций с **actionable-кнопками**: Ack, Mute 1h, Escalate, Explain (триггерит AI).

Что сделать:
- Новые нотифаеры в `notifier/pkg/notifier/*` по образцу существующих.
- Webhook от Telegram/Slack обратно в RR через публичный API для handle-кнопок.
- В UI — новый тип интеграции с настройками бота/webhook URL.

Value: radically сокращает TTR (time to respond). У проекта уже есть Telegram-группа сообщества — продуктовый message «мы там, где ваши команды».

Усилие: M (1 неделя).

---

### 5. Honeypod detector — canary workloads

Декларативно размечаешь deployment/pod лейблом `runtime-radar.io/honeypod=true`. Любой `process_exec` внутри — high-severity incident с auto-escalate. Настраивать детекторы/фильтры не нужно — сам факт запуска процесса = угроза.

Что сделать:
- Фильтр по label в `runtime-monitor` (Tetragon `podSelector`).
- Preset-правило в `policy-enforcer` с severity=critical.
- В UI — galочка «Honey workload» или отдельная вкладка.

Value: низкий false-positive rate, мощный сигнал компрометации, уникальный security-фичер для демо/пресейла.

Усилие: S–M (3–5 дней).

## Часть 2

### 6. Auto-remediation — автоматические containment-действия

На критичных детекциях запускать действие: `kill` процесса через Tetragon enforcement, изоляция пода (навесить `NetworkPolicy deny-all`), `cordon` ноды, scale-down deployment. Конфигурируется в правилах ответа рядом с нотификациями.

Что сделать:
- Расширить `policy-enforcer` новым типом действия (поле `response_action` у правила).
- Для kill-процесса — Tetragon enforcement hooks в `runtime-monitor`.
- Новый контроллер `remediator` для K8s-действий (NetworkPolicy, cordon).
- Обязательный dry-run режим и RBAC-ограничения на применение.

Value: разница между «обнаружили и написали в Slack через 3 минуты» и «сработало за 200 мс» — критическая при live-атаке.

Усилие: L (2–3 недели).

---

### 7. MITRE ATT&CK mapping + coverage dashboard

Каждый детектор размечается тактиками/техниками MITRE (T1059 Command and Scripting Interpreter, T1055 Process Injection и т.д.). В UI — тепловая карта покрытия (какие техники ловим, какие нет) и в инциденте — привязанная техника с ссылкой на MITRE.

Что сделать:
- Добавить поле `mitre_techniques []string` в `model.Detector`.
- UI-страница coverage (heatmap по тактикам ATT&CK).
- Разметка существующих детекторов (разовая ручная работа).

Value: обязательное требование enterprise-заказчиков и compliance-аудитов. Стандартный язык общения security-команд.

Усилие: M (1 неделя + разметка).

---

### 8. Baseline anomaly detection

В первые 7 дней после деплоя workload собираем «обычный» граф процессов и сетевых соединений. Дальше — алерт на отклонения: новый бинарник в поде, новое исходящее подключение, нестандартный родитель процесса.

Что сделать:
- Новый сервис `baseline-learner` или расширение `event-processor`.
- Хранение профилей workload в ClickHouse.
- Новый тип детектора `anomaly` в `policy-enforcer`.
- UI для тюнинга baseline (подтвердить/исключить наблюдение).

Value: ловит zero-day и специфику, которую сигнатурные детекторы пропустят.

Усилие: L (3–4 недели). Основной риск — false-positives в первые недели.

---

### 9. AI usage & cost dashboard

Продолжение фичи №1. Показать: сколько токенов потрачено per integration, средняя задержка, оценка стоимости (по прайсу OpenAI/Anthropic), rate-limit статус.

Что сделать:
- Prometheus-counters в `notifier` на каждый AI-вызов (tokens, duration, status).
- Страница в UI «AI usage».
- Опционально — Grafana dashboard в helm-поставке.

Value: без этого при росте нагрузки AI-интеграции превращаются в чёрный ящик с непредсказуемым счётом.

Усилие: S–M (3–5 дней).

---

### 10. Investigation timeline + post-mortem export

Продолжение фичи №3. Для инцидента собираем полный timeline: события → AI-объяснения → действия аналитика (ack/mute/escalate) → auto-remediation → комментарии. Экспорт в Markdown/PDF одной кнопкой.

Что сделать:
- Новый эндпоинт `history-api`: `/incident/{id}/timeline`.
- Рендер PDF через `chromedp` или аналог.
- AI-генерация executive summary в шапку отчёта.

Value: post-mortems — обязательный артефакт, сейчас пишутся вручную часами. Для демо продаж — «вот так выглядит готовый отчёт после инцидента».

Усилие: M (1–1.5 недели).
