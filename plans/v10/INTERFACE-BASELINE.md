# Интерфейс: наблюдённый B1 baseline и контракт T01A

Ниже B1 проверен 24 сентября 2026 по source
`cbf5ee767ec1a64bab5085deba319a4c5b066203`, содержащему qualified B1
`0e49b4fd…`. Это исторический наблюдённый baseline. Выбранный T01A контракт
отделён ниже; его runtime результат устанавливают код и E2.01, а не эта таблица.
Основание выбора — [D1](B2-DESIGN-DECISIONS.md#d1-пять-инструментов-по-задачам-один-application-core).

B1 MCP рекламировал **один tool `haft`**, публичный envelope `haft.api/2`.
У CLI `haft10 api` та же доставка; внутренний app request — `haft.api/1`.
Все операции B1 разделяли одну широкую schema и консервативные annotations.

| Operation | Action | Эффект и смысл |
|---|---|---|
| `remember` | omitted, `terms` | Публикация carrier/terms; retained parts по refs |
| `recall` | omitted, `legacy` | Точное чтение/поиск и legacy-представление |
| `context` | omitted | Контекст записей и кода на captured basis |
| `impact` | omitted | Ограниченный разбор затронутых связей |
| `fpf` | omitted=`status`, `search`, `inspect` | Чтение источника |
| `source` | omitted=`status`, `search`, `inspect` | Тот же source handler, совместимый alias |
| `check` | omitted=`structural`, `prepare`, `observe` | Проверка структуры/основы, классификация переданного результата; не самостоятельный запуск tests |
| `change` | `create`, `list`, `show`, `preview`, `apply`, `sync`, `archive`, `reopen`, `rebase`, `update` | Чтения и записи change; конкретный action определяет эффект |
| `recover` | omitted | Запись: закончить допустимую незавершённую публикацию; конфликт не overwrite |
| `read` | omitted | Progressive delivery: summary/detail/parts/bytes, digest/generation/cursor |

`source` и `fpf` — два enum spelling, не два независимых механизма. `recover`
не является read-only. `read` обрабатывается delivery boundary; его отсутствие
в app switch не означает отсутствие публичной операции. У claim поле `checks`,
а не `checked_by`; `implemented_by` — место реализации, не свидетельство pass.

Источники наблюдённого B1, с которыми T01A сверяет единый каталог:
[MCP](../../internal/core/transport/mcp.go),
[schema](../../internal/core/transport/json.go),
[app](../../internal/core/app/types.go),
[delivery](../../internal/core/app/delivery.go),
[claim](../../internal/core/carrier/types.go).
Эта таблица — зафиксированный baseline; после T01A документация и schema checks
сверяются с тем же каталогом, который использует implementation.

## Выбранный интерфейс T01A

`haft10 serve` выбирает default MCP profile с пятью task tools;
`haft10 serve --profile legacy` выбирает только совместимый `haft` с закрытой
schema из того же action catalog. Оба профиля
используют публичный `haft.api/2` и один application core. CLI
`haft10 api --input FILE|-` сохраняет его и исполняет JSON из возвращённого
`next_request` или запроса именованной части без изменения аргументов.
После B1 upgrade `haft10 init --codex` обновляет точные generated Codex assets;
редактированные generated assets вызывают preflight conflict.
Совместимость означает прежний смысл
логического запроса, receipt и digest; она не означает рекламу двух каталогов
одному host одновременно.
Frozen B1 replay в T01AR охватывает одну note, а проверки terms/change/retain
выполняют новые записи кандидата и не доказывают их исторические B1 bytes.

| Default MCP tool | Допустимые B1 operation spellings | Предел эффекта |
|---|---|---|
| `haft_read` | `recall`, `context`, `impact`, `read` | Чтение canonical records, включая `recall/legacy` и progressive continuation; возможна запись lock/cache |
| `haft_write` | `remember` | Публикация carrier или terms; retain по captured ref |
| `haft_change` | `change`, `recover` | Mixed read/write; `recover` явно пишет |
| `haft_check` | `check` | T01A structural/prepare/observe; tests не запускаются; observe принимает optional `code_config` |
| `haft_fpf` | `fpf`, `source` | Pinned-source read; нет fetch/pin mutation; возможна запись cache |

| Profile/tool | `readOnlyHint` | `destructiveHint` | `idempotentHint` | `openWorldHint` |
|---|---|---|---|---|
| default `haft_read`, `haft_check`, `haft_fpf` | false | false | false | false |
| default `haft_write`, `haft_change` | false | true | false | false |
| legacy `haft` | false | true | false | false |

Каждый tool принимает `format` и `operation`; собственная schema ограничивает
operation, action, поля и обязательные сочетания. Для non-default ветви
`action` обязателен; пропуск разрешён лишь для существующего default app action.
Единый [каталог](../../internal/core/transport/catalog.go) питает обе MCP schema,
CLI help и генерируемые skills; его согласованность проверяют catalog/protocol tests.
Неизвестные управляющие поля и wrong-tool operation отвергаются до эффекта.
Optional nil-able поля, включая `snapshots:null`, принимаются как omission;
required-null и scalar-null отвергаются, как и поле вне своей action-ветви.
Unknown authored extensions в carrier/claim сохраняются. Аннотации отражают
максимальный возможный эффект tool на environment, отдельно от canonical
publication, и не служат разрешением. Даже canonical read может создать
`.haft/.runtime/writer.lock` и disposable `.haft/.cache/disclosure` entry.
Эти additive записи объясняют `readOnlyHint=false` без destructive publication
для read/check/fpf. Повторный вызов может захватить новую основу или записать
новый transient result, поэтому `idempotentHint=false` консервативен.
Fresh store read требует writable `.haft/.runtime`; transient continuation
требует writable `.haft/.cache/disclosure`. При отказе cache публикации ответ
обозначает continuation unavailable, не меняя committed receipt; при отказе
lock соответствующее store-действие unavailable. T01 пересмотрит модель
эффектов `haft_check` после добавления исполнения tests.

Ответ остаётся bounded `haft.api/2`: `result_kind`, `data`, `diagnostics`,
`basis`, `coverage`, `limits`, `is_error`, `delivery`. `delivery.read_tool`
указывает реально рекламируемый инструмент для каждого `next_request` и
запроса именованной части: `haft_read` в default, `haft` в legacy. Аргументы
read request остаются теми же; вызов выполняется через указанный tool.
Полный serialized MCP result, включая error/text/structuredContent и binding,
не превышает 8192 bytes. Summary → named parts → continuation объясняют
initialize instructions и tool/result descriptions без установки skills.
Код каталога должен давать те же operation/action/schema/effect/example данные
для help и generated skills; E2.01 проверяет это на реальном MCP и CLI.

## Файлы и вычислимые данные

| Класс | Текущая роль |
|---|---|
| `.haft/<kind>/*.md` и changes | Читаемые canonical carriers |
| `.haft/editions/sha256/*.json` | Обязательные immutable snapshots с carrier bytes и interpretation |
| `.haft/transactions/` | Manifest/outputs/commit marker для replay и восстановления; не disposable cache |
| `.haft/.runtime/` | Locks и временная публикация; не переносимая память |
| `.haft/.cache/disclosure/` | JSON transient results; потеря даёт expired, а не поддельный полный ответ |
| SQLite | Только legacy v9 import в `internal/core/migrate`; нового SQLite index в B1 нет |

Полные captured experiment directories дополнительно содержат outputs, copies
и Go build cache. Их общий размер нельзя назвать размером Haft database.
