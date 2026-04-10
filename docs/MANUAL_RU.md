# MezaMozg: Мануал (RU)

## 1. Что уже реализовано

- `Meza-Core` (Go): API, задачи, аудит, AI-планирование, file persistence.
- `Meza-Panel` (Next.js + shadcn/ui): вкладки, рабочие кнопки, чат AI-агента.
- `Meza-Node` (Rust scaffold): заготовка агента.

Панель переведена на русский и работает через серверные proxy-роуты `/api/*`, поэтому операторский токен не уходит в браузер.

## 2. Быстрый запуск локально

Из корня репозитория:

```bash
cp .env.example .env
```

Запуск backend:

```bash
cd apps/meza-core
MEZA_CORE_DATA_PATH=/tmp/mezamozg/state.json \
MEZA_OPERATOR_TOKEN=prod-operator-token \
MEZA_BOOTSTRAP_TOKEN=prod-bootstrap-token \
MEZA_NODE_TOKEN=prod-node-token \
GOCACHE=/tmp/meza-go-cache \
go run ./cmd/api
```

Запуск frontend (в новом окне терминала):

```bash
cd apps/meza-panel
npm install
MEZA_CORE_BASE_URL=http://127.0.0.1:8080 \
MEZA_PANEL_OPERATOR_TOKEN=prod-operator-token \
npm run dev -- --hostname 0.0.0.0 --port 3000
```

Открыть:

- `http://localhost:3000`

Для docker-режима прод-доступа панель поднимается по HTTPS на `https://localhost:1499` с self-signed сертификатом.

## 3. Вкладки панели

- `Обзор`: сводные метрики и статус инфраструктуры.
- `Ноды`: список серверов, теги, CPU/RAM.
- `Задачи`: форма создания + список задач с `Подтвердить`/`Запустить`.
- `SSH Терминал`: ручной режим выполнения команд на выбранной ноде со стримингом вывода в реальном времени (SSE).
- `AI Агент`: чат, где оператор пишет команду обычным языком.
- `Алерты и аудит`: автоматические алерты и журнал действий.

## 4. Проверка, что кнопки работают

### Создание задачи

1. Открой вкладку `Задачи`.
2. Заполни поля формы.
3. Нажми `Создать задачу`.
4. В списке появится новый `job-*`.

Поддерживаемые типы в форме:
- `shell_command` (payload: `{ command: "..." }`)
- `bash_script` (payload: `{ script: "..." }`)

### Подтверждение и запуск

1. Для задачи в статусе `awaiting approval` нажми `Подтвердить`.
2. Нажми `Запустить`.
3. Статус перейдёт в `completed` (для текущей симуляции rollout).

### Кнопка “Алерты”

- В шапке нажми `Алерты`.
- Панель переключится на вкладку `Алерты и аудит`.

### Кнопка “Создать задачу”

- В шапке нажми `Создать задачу`.
- Панель переключится на вкладку `Задачи`.

## 5. AI-агент (чат)

Пример запроса:

`обнови docker на ноде argentina-17`

Что происходит:

1. Панель отправляет запрос в `/api/agent/chat`.
2. `Meza-Core` строит план и создаёт типизированную задачу.
3. Если операция рискованная, задача создаётся в `awaiting_approval`.
4. После подтверждения оператор может запустить rollout.

Опция `Автозапуск` в чате запускает только те задачи, которые не требуют approve.

## 6. Сборка и тесты перед деплоем

Frontend:

```bash
cd apps/meza-panel
npm run build -- --webpack
```

Backend:

```bash
cd apps/meza-core
GOCACHE=/tmp/meza-go-cache go test ./...
```

## 7. Деплой через Docker Compose

```bash
cp .env.example .env
docker compose up --build
```

Сервисы:

- `meza-core`: `http://127.0.0.1:8080`
- `meza-panel`: `https://127.0.0.1:1499` (self-signed SSL)

При первом открытии браузер покажет предупреждение сертификата, это ожидаемо для self-signed.

## 8. Установка ноды без ручных действий

Единый one-line инсталлер (как ты просил):

```bash
curl -fsSL https://raw.githubusercontent.com/ASTRACAT2022/MegaMozg/main/scripts/install.sh | bash -s -- -install hub
curl -fsSL https://raw.githubusercontent.com/ASTRACAT2022/MegaMozg/main/scripts/install.sh | bash -s -- -install node <HUB_IP> <BOOTSTRAP_TOKEN>
```

Если порт `8080` на хосте занят, инсталлер хаба автоматически выберет другой порт для `meza-core` (например `18080`), а панель останется на `1499`.

Старый локальный скрипт тоже доступен:

One-line установка:

```bash
bash scripts/install-node.sh --token prod-bootstrap-token --core-url http://127.0.0.1:8080 --region ru-central --tags docker,prod
```

Что делает скрипт автоматически:

- Регистрирует ноду в хабе.
- Ставит heartbeat-агент как сервис (systemd) или фоновый процесс.
- Сразу запускает авто-подключение к хабу без дополнительных кликов.

### Примечание по режиму `local_exec` в терминале

По умолчанию включён безопасный `job_simulated`.
Если нужен режим выполнения команды на самом хосте `meza-core`, выставь:

```bash
MEZA_TERMINAL_LOCAL_EXEC_ENABLED=true
```

## 9. Мини-чеклист “готово к проду”

- Установлены реальные токены (не дефолтные).
- Закрыт доступ к panel/core на уровне сети (security group / firewall).
- Включены резервные копии `MEZA_CORE_DATA_PATH` или подключена БД.
- Настроен HTTPS и reverse proxy (Nginx/Caddy/Ingress).
- Определён процесс обновления `meza-core` и `meza-panel` без downtime.
