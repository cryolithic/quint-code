# Haft v10: контракт носителей и связей

Дата основы: 23 сентября 2026; уточнение baseline/плана — 24 сентября;
контракт T01A и T01B — 25 сентября. **Ниже сохранена семантика B1 и отдельно заданы
выбранные изменения T01A/T01B.** [B2-DESIGN-DECISIONS](B2-DESIGN-DECISIONS.md), D1–D6,
имеют приоритет для новых writers в соответствующих тикетах. §5.10 описывает
исторический B1 delivery baseline; §5.11 задаёт T01A, §5.13 задаёт T01B.
Другие deltas B2A не становятся реализованными от этого уточнения: `haft/1`
остаётся B1. Public release, live migration и переинтерпретация
старых snapshots этим не утверждаются.

[DECISIONS-LOG.md](DECISIONS-LOG.md) сохраняет приоритет с последующим уточнением
[политики миграции](MIGRATION-POLICY-2026-09-23.md), которое заменяет только прежний
безусловный запрет перехода при пропуске decision/note/spec. Дополнительные прямые
уточнения — [PRODUCT-DIRECTION](PRODUCT-DIRECTION-2026-09-23.md): имя `implemented_by`,
точные связи с кодом и цель покрытия OpenSpec. Здесь сведены исправления
[ревью](REVIEW-SPECS-RELATIONS-2026-09-23.md) и последующей проверки самого контракта.
SPECS.md и RELATIONS.md остаются исследовательским обоснованием, v10-draft.ru.md —
историей. Форматы реализации берутся отсюда; порядок работ и приёмка — из WORKPLAN.md;
отображение старых данных — из MIGRATION-MAP.md; эффекты, конкурентность и восстановление —
из RUNTIME-CONTRACT.md. Черновики не добавляют операторского принятия нового продукта,
выпуска, несовместимого перехода или очистки данных.

FPF не предписывает эти поля или число типов связей. Это локальный формат Haft,
использующий различения C.2.1, A.6.B, A.10/A.2.4, C.11, A.6.6 и A.6.0. Схема проверяет
структуру; агент и человек устанавливают смысл, применимость и достаточность основания.
Не возвращаются компилятор FPF-типов, универсальный workflow, числовое доверие или
жизненный цикл 9.x. Принятие, актуальность, истинность и полномочие различаются.

## 1. Общее для всех записей

Текущий носитель — Markdown-файл `.haft/<вид>/<id>.md`. Frontmatter несёт структурные
поля; тело несёт читаемое содержание, включая наблюдения и обоснование. Парсер не
извлекает из тела автоматические управляющие связи. Это не делает тело бессмысленным:
его байты входят в точный снимок редакции; поиск может индексировать его текст.

| kind | Префикс ID | Каталог |
|---|---|---|
| decision | dec | `.haft/decisions/` |
| spec | spec | `.haft/specs/` |
| note | note | `.haft/notes/` |
| problem | prob | `.haft/problems/` |
| evidence | ev | `.haft/evidence/` |
| options | opt | `.haft/options/` |

```yaml
format: haft/1
id: dec-20260923-3f9a1c7e
kind: decision                     # decision | spec | note | problem | evidence | options
title: ...
status: proposed                   # proposed | active
origin: agent_proposal             # agent_proposal | operator_request | migrated_9x
about: dir:internal/ingest
created_at: 2026-09-23T10:00:00Z
updated_at: 2026-09-23T10:00:00Z
reopen_when: ...                   # условное поле, текст; дата допустима
write_receipt:                     # optional idempotency, §2.3
  request_id: ...
  payload_digest: sha256:<64hex>
links: []
sources: []
```

### 1.1. Имена, статусы и принятие

- Формат ID `dec-YYYYMMDD-8hex` и аналогичные префиксы сохраняется по выбору Ивана.
  Суффикс генерируется криптографически, но **не является хэшем содержания или гарантией
  уникальности**. Создание проверяет конфликт ID и не перезаписывает существующий файл.
  Дубликат после Git merge — диагностируемый конфликт, не случайный победитель.
- Авторских статусов ровно два: proposed и active. Для decision/spec active представляет
  принятое содержание в доверенной модели и требует `operator_confirmed: true` с честным
  `origin: operator_request`. Агент не проставляет подтверждение из собственного
  предложения, цитаты или успешной проверки. Это правило относится к `haft/1`;
  для `haft/2` spec действует §5.13, decision по-прежнему использует `haft/1`.
- Перевод proposed decision/spec в active и изменение действующего содержания выполняются
  новой записью через supersedes, в baseline B1. Выбор Ивана №2 сохраняется для decision; применение к spec
  уточнено будущим D2. Обычная правка YAML в B1 не считается
  выполненной процедурой принятия. При прямом доступе к файлам ядро не доказывает, кто
  поставил флаг; README объясняет этот предел.
- Для note/problem/evidence/options active означает текущую запись своего вида, а не
  одобрение нормы, истинности или решения.
- Superseded, конфликт голов, историческая неактивность и неразрешённость — результаты
  проекции, не новые значения status. Импорт сохраняет legacy_status и происхождение;
  прежняя неактивность не превращается в действующую норму. Импортная проекция определена
  в MIGRATION-MAP; вопрос о применении исторических active ограничений открыт в §10.
- Origin и operator_confirmed сами являются утверждениями доверенной модели. Статус
  не создаёт полномочие и не устанавливает применимость к текущей работе.

### 1.2. Предмет about

Один восстанавливаемый предмет: `file:<путь>`, `dir:<путь>`, `sym:<путь>::<имя>`,
`system:<имя>`, `domain:<квалифицированный термин>`, ID записи или `spec:<slug>`.
Локальные адреса разрешаются в пределах проекта; одинаковые токены другого проекта не
считаются теми же ссылками. Пути относительны корню проекта.

Неоднозначный символ уточняется языковой сигнатурой и полным digest её представления.
Короткое имя/8hex в UI не служит идентичностью. Переименование файла, изменение символа
и смена описываемого предмета — разные случаи. Точную привязку и legacy-кандидата
показывают раздельно; сходство имени не доказывает тождество.

About — указание предмета, не доказательство C.2.1-конституции и не автоматическая область
ограничения. Интервью про модуль может иметь about dir. Неизвестный или составной
исторический предмет не заменяется вымышленным system:haft: содержание сохраняется с
причиной неполной структурной проекции.

### 1.3. Неизвестные данные

Неизвестные поля сохраняются без потерь. Неизвестные format/kind, невалидный status,
дубликаты YAML-ключей, конфликт ID и неразрешимые обязательные поля не допускаются в
действующую управляющую проекцию. Носители доступны для чтения/поиска с диагностикой и
учитываются в coverage. Unknown format нельзя интерпретировать как знакомую норму
только из-за полей active и constrains.

## 2. Адреса и сохраняемые редакции

**Живой адрес** `dec-...`, `spec:order-cancel` или `spec:order-cancel#L1` запрашивает
текущую проекцию. **Закреплённый адрес** содержит полный SHA-256 сохранённого снимка:
`<record-id>@sha256:<64hex>`; адрес claim: `<spec-id>@sha256:<64hex>#<claim-id>`.
Сокращённый digest используется только для показа.

### 2.1. Минимальный неизменяемый снимок

Haft хранит под Git `.haft/editions/sha256/<64hex>.json`. Это материал для разрешения
выданных исторических ссылок, а не disposable cache. Один снимок содержит:

- `format: haft.snapshot/1`;
- полные исходные байты Markdown-носителя в base64, включая body, links, status,
  timestamps, write_receipt, legacy и неизвестные поля;
- версию локальных правил интерпретации и адресации;
- точные байты выбранного term-map и других явно объявленных локальных оснований
  интерпретации, когда они нужны, либо явное отсутствие такого основания.

B1 wire envelope uses exactly `format`, `carrier_bytes_base64` and
`interpretation: {version, namespace, terms, other}` in that order. Version is
`haft.interpretation/1`; absent terms are JSON null. A basis file is
`{name, bytes_base64}`; other bases are sorted by unique name. JSON is compact
with one final LF; all byte arrays are base64. Reader hashes the original JSON,
rejects duplicate keys, and does not reinterpret an unknown version. Source
snapshots use `format`, `source_bytes_base64`, `provenance`; provenance omits its
own snapshot_ref to avoid self-reference. Short/old migration IDs are retained in
legacy provenance and a source-ID lookup mapping by the migration adapter; they
are never silently admitted as new native IDs or redirected without the mapping.

Digest считается по полным байтам сохранённого snapshot-файла. Writer использует одну
стабильную сериализацию с фиксированными golden-примерами. Reader проверяет digest
исходных байтов, а не повторно сериализованного JSON. Разное форматирование может дать
два снимка одного смысла: допустимая переплата за точность, не заявление о двух
FPF-эпистемах. Контентный адрес обозначает сохраняемый representation snapshot, а не
заменяет онтологическое правило C.2.1.

Ничего не исключается «как несемантическое»: связь, статус или пояснение способны менять
используемое утверждение. Snapshot не содержит собственный digest. Base64 сохраняет и
исторические не-UTF-8 байты; такие носители не становятся валидными live-записями.
Source SHA остаётся SHA действительно прочитанного источника, а не новым текущим HEAD.

### 2.2. Интерпретация терминов и claims

У claim стабильный id в пределах секции. Удалённый ID не переиспользуется для другого
утверждения. Claim разрешается после точной редакции секции; одинаковый L1 в двух
снимках не переносит поддержку.

Зависимости от терминов задаются явно в terms секции и, когда нужна точность, в terms
отдельного claim. Поиск подстроки не устанавливает семантическую зависимость. В первой
реализации допустимо сохранять весь небольшой term-map и при изменении показывать
консервативного кандидата на пересмотр. Это расширенная рамка поиска, не вывод «все
claims изменили смысл». Отсутствие совпавшего слова не доказывает независимость.

Term-map не исчерпывает effective reference scheme: формат, namespace, определения,
явно заявленные условия тоже сохраняются, когда меняют интерпретацию. Компилятор
reference schemes не нужен. Snapshot сохраняет выбранное основание, но не доказывает
полноту неявных смысловых зависимостей.

### 2.3. Запись, закрепление и восстановление

Remember создаёт новую запись или преемника, не перезаписывает существующий ID. Агент
передаёт live target; операция читает точные bytes/basis, сохраняет immutable snapshot,
затем публикует новый носитель с закреплённой ссылкой. Перед публикацией проверяются
ожидаемый digest/голова: устаревший запрос получает конфликт, а не новые незамеченные байты.

Для live evidence target и live options, из которого уже сделан выбор, запрос обязан
нести `expected_target_digest` из предварительного recall/query. Чтение может вернуть
digest без записи snapshot; write сохраняет именно проверенные bytes, если digest всё
ещё совпадает. Последовательность «прочитали A; другой агент записал B; evidence про A»
не закрепляет B: возвращается conflict. Готовый pinned адрес разрешается по своему
snapshot без переназначения на текущую редакцию. Одно лишь наличие файла во время write
не доказывает, что наблюдение или выбор относились к нему.

Новый носитель публикуется no-clobber после подготовки snapshots. Predecessor не
переписывается ради supersession. Невостребованный snapshot после сбоя безопасен;
ссылка на ещё не сохранённый snapshot недопустима. Два rename не называются атомарной
транзакцией. Подробный протокол и восстановление — RUNTIME-CONTRACT.md.

Процессы разделяют краткую блокировку записи, а не запрет второго MCP. Прямой внешний
редактор/Git не участвует в этом lock: стабильный наблюдённый scan не равен атомарному
снимку файловой системы. При движении дерева — ограниченные повторы и degraded, без
заявления несостоявшейся целостности. Уже выданные pinned refs остаются разрешимыми из
сохранённых bytes независимо от позднейших live-правок. Не гарантируется запись каждой
никогда не закреплявшейся промежуточной ручной редакции. При отсутствии snapshot —
unresolved, никогда не live fallback. Автоматической очистки editions в первой версии нет.

Optional request_id запроса позволяет повторить remember после потери ответа. Носитель
сохраняет один write_receipt с request_id и полным payload_digest нормализованного
исходного запроса до server-generated ID/time и разрешения live refs. Тот же ключ и
payload возвращают прежний результат; иной payload — request_conflict. Сравнение
запроса отдельно от digest snapshot. Write_receipt входит в bytes снимка; отличие
snapshot из-за него не объявляется смысловым различием. Без request_id автоматический
повтор после неизвестного исхода не обещается; сначала проверяется результат.

Закрепление обязательно для evidence target и выбора из отдельного options; для
relies_on — когда требуется восстановление именно прежнего основания. Slug и aliases
уникальны между независимыми линиями секций, а не между файлами одной линии supersession.
Преемник может сохранять alias своего предшественника; unrelated запись с тем же alias
даёт конфликт. Живой alias выбирает только действующую голову этой линии; proposed не
перехватывает alias active-предшественника, а две active-головы возвращают конфликт.
При отсутствии active содержимое proposed доступно явно как предложение, без принятия.
Pinned адрес не зависит от этого разрешения и не переназначается при переименовании.

## 3. Замена записей и текущие головы

Каноническое supersedes — список точных адресов предшественников. Обычно один элемент;
несколько нужны при принятии proposal поверх действующего основания либо явном
разрешении конфликтующих ветвей одной линии. Вместе с ним обязательна читаемая
supersede_reason. Writable superseded_by не существует.

- Один kind — необходимое локальное ограничение, не доказательство продолжения.
  Ordinary supersession сохраняет предмет, а у decision — вопрос выбора. Изменённые
  claims/обоснование/схема должны объясняться как продолжение этой линии. Существенно
  другой предмет или вопрос создаёт новую запись с навигацией; retarget_reason не
  обходит это правило.
- Валидатор проверяет refs, kind, точные predecessor editions, отсутствие цикла,
  ожидаемое множество голов и поля. Он не доказывает семантическое тождество вопроса
  или достаточность причины: это суждение автора/оператора, без NLP-судьи.
- Proposed-преемник виден как предложенная замена и не гасит действующую запись.
  Принятый active-преемник заменяет указанную текущую голову в той же области.
- При принятии предложения явно перечисляются само предложение и действующие головы,
  которые оно заменяет. Пример: A active; B proposed supersedes [A]; оператор принимает
  содержание B; новая C active supersedes [A, B]. Если это первая запись без active
  основания, C supersedes [B]. Ядро проверяет snapshot B и ожидаемый набор active
  оснований. C supersedes только [B] при сохраняющейся A не публикуется как успешная
  замена A: возвращается точный missing_predecessor/conflict. Транзитивного скрытого
  принятия или погашения любых предков нет.
- Два active-преемника — конфликт. Recall/context возвращают обе ветви и ограничение
  использования. Timestamp, порядок файлов и новый ID не выбирают истину. Нельзя
  маскировать конфликт общим «непротиворечивым governing set».
- Разрешение — новая запись, явно перечисляющая все заменяемые головы; для decision/spec
  нужен текущий операторский выбор. CAS проверяет неизменность набора голов. До этого
  unrelated work продолжается, затронутый вывод остаётся contested; ветвь не удаляется.
- У спеки одинаковый claim ID помогает сопоставлению, но не доказывает сохранение
  смысла, harness fit или перенос evidence.

Наличие supersedes не доказывает FPF EpistemeEditionRelation и не создаёт полномочие.

## 4. Виды записей

### 4.1. decision

Минимум Ивана, использующий C.11; заполнение не гарантирует полного соответствия C.11.
Обязательны object, question, why, а при choose_now — chosen. Есть одна отвергнутая
альтернатива с причиной либо no_alternative_reason. При отсутствии альтернатив выбранный
вариант не исчезает.

```yaml
disposition: choose_now           # choose_now | reject_current_set | probe_again | reroute
chosen: kafka
options:
  - {id: nats, verdict: rejected, reason: нет опыта эксплуатации}
why: используем имеющуюся эксплуатационную основу при нужной гарантии доставки
choice_rule: ...                  # условно; правило может быть выражено в why
weakest_link: ...                 # условно
invariants: []                    # условно
predictions: []                   # условно
operator_confirmed: true          # для принятой active-записи
constrains: [dir:internal/ingest]  # только choose_now
```

Reject_current_set сохраняет отвергнутый набор и причину; probe_again — конкретный probe;
reroute — reroute_to. Эти результаты не требуют фиктивного chosen и не несут constrains.
Это разные формы данных, а не несовместимые boolean-флаги. Валидатор проверяет форму,
но **не решает, объясняет ли why выбор достаточно хорошо**. Advisory-подсказку по смыслу
может дать reviewer/skill; число вариантов не заменяет осмысленное правило сравнения.

### 4.2. spec

Форма в §5. Kind spec — локальный контейнер, не гарантия Spec-use по E.10.D2.

### 4.3. note

Общие поля и читаемое содержание. Constrains запрещён. Kind не делает наблюдение
проверенным фактом.

### 4.4. problem

Object и question обязательны; signal и acceptance условны. Проблема не является
обещанием её решить или разрешением на работы.

### 4.5. evidence

Наблюдение/результат и отдельные применения в §6.

### 4.6. options

Условный проектный вариант для набора без выбора. Не создаётся, если достаточное
сравнение уже есть в другой записи. Next_use называет конкретное будущее использование.
Это пока не объявлено продуктовым принятием открытого вопроса 7.

```yaml
kind: options
question: какой брокер использовать
options:
  - {id: kafka, summary: ..., weakest_link: ...}
  - {id: nats, summary: ..., weakest_link: ...}
comparison:
  characteristics: [ops_cost, at_least_once, team_experience]
  basis: ...
  comparator: ...
  non_dominated: [kafka, nats]
next_use: probe_again             # probe_again | shortlist | reopen_replay
probe: ...
```

Comparison условен; non_dominated требует basis/comparator, но их заполненность не
доказывает Pareto-свойство. Haft его не вычисляет. Unknown и несопоставимость видимы.
Решение закрепляет точный snapshot набора; копировать живой набор в два конкурирующих
места не требуется.

### 4.7. term-map

Вспомогательный `.haft/specs/terms.md`, не седьмой kind и не вся reference scheme.
У термина стабильный квалифицированный ID, definition, optional aliases и исключения.
Канонический frontmatter вспомогательного файла:

```yaml
format: haft.terms/1
terms:
  - id: Billing.Order
    definition: Domain order with an accounting total.
    aliases: [order]
    exclusions: [invoice]
```

Это отдельный parser для вспомогательной основы, не kind record. Unknown fields
и body сохраняются. Одинаковое слово в разных доменах получает разные ID. Проверяется разрешимость явно
указанных term refs, но не достаточность их набора (§2).

## 5. Спека

### 5.1. Секция и спецификационное использование

Носитель `.haft/specs/<id>.md` содержит subject, slug, terms, claims, receiving_use и
conditional constrains. E.10.D2:4.1.3 требует восстанавливаемой эпистемы, достаточно
определённых checkable claims и названного harness/validation relation, **способного
проверять их для stated use**; viewpoint-ветка условна. Receiving_use помогает это
объяснить, но получатель не заменяет остальные условия.

```yaml
kind: spec
slug: order-cancel
about: domain:Billing.OrderCancellation
receiving_use: проверка сохранения total перед изменением Cancel
terms: [Billing.Order, Billing.PaidOrder]
constrains: [file:internal/order/cancel.go]
claims:
  - id: L1
    kind: law
    text: Cancel сохраняет total заказа
    implemented_by:
      - ref: sym:internal/order/cancel.go::Order.Cancel
        covers: переход доменной модели; HTTP-обработчик сюда не входит
    checks:
      - ref: pbt:internal/order/cancel_test.go::TestCancelPreservesTotal
        covers: генерация new и paid; другие состояния и HTTP-путь не проверяются
  - id: L2
    kind: definition
    text: отменяемый заказ имеет статус new или paid
  - id: A1
    kind: guard
    text: Cancel допускается только для отменяемого заказа
    refs: [L2]
    unchecked: нет проверки HTTP-входа
```

Checks могут быть живыми адресами будущей проверки. Факт выполненной проверки закрепляет
версии цели и проверяющего средства в evidence. Examples given/when/then и open условны;
пример не вводит вторую модель, открытый вопрос не создаёт автоматический human gate.

### 5.2. Виды утверждений

| kind | Квадрант | Предикат |
|---|---|---|
| definition | L | значение понятия в объявленной модели/схеме |
| law | L | инвариант, свойство или закон модели |
| guard | A | условие допуска конкретного применения на входе |
| prescription | D | норма/обязательство с источником и носителем, когда это нужно |

Kind выбирается по смыслу, не по «нельзя», «должен» или given/when/then. «Из shipped
нет перехода» может быть law; «операция допускается при…» — guard; «политика запрещает
оператору…» — prescription. Неясный смешанный предикат уточняет автор, не валидатор.

E-результаты живут в evidence. Это форма носителя v10, не запрет FPF на разные квадранты
в одной публикации. Definition не требует искусственного исполняемого теста; law с
unchecked ещё не устанавливает Spec-use этой claim.

### 5.3. Зависимости утверждений

Refs обозначает смысловую зависимость, не любое упоминание. Локальный ID разрешается
внутри точной редакции; межсекционная зависимость использует адрес §2. Запреты A.6.B:6.4
для этих видов: L→A/D, A→D. Внутри квадранта ссылки допустимы; D может ссылаться на L/A/D.
Informative-исключение не автоматизируется: поясняющая цитата остаётся в теле без
автоматической dependency-семантики.

A может потреблять отдельно установленный E-результат, нужный предикату: conditional
поле evidence_inputs с адресом конкретного use/результата evidence и его применимостью.
Это не превращает evidence в claim спеки. L не получает нормативную зависимость от E;
E не обосновывается нормой D как доказательством выполнения. Эти входы не нужны каждой
секции, но проверяющий не должен объявлять все A→E незаконными.

### 5.4. Harness

Checks содержит ref и обязательный covers: что проверяется и с какими пределами.
Условия исполнения и interpretation basis добавляются, когда меняют пригодность.
Типы адресов: test, pbt, type, contract, lint, arch, gate, model, manual. Доступность
или имя проверки не доказывают способность проверять данное утверждение.

Unchecked с причиной — честное описание пробела. Для law/guard check --strict требует
checks либо unchecked, проверяет структуру covers/refs, но не запускает проверки, не
оценивает их смысл, правдивость или полноту. Результаты разделены: carrier_valid,
диагностика разрешимости, validation_basis_declared/absent. Green schema check не
называется «спека проверена». Объявленный путь не приравнивается к пригодному harness.

Пригодность проверяется в ранней пробе: полезная проверка, существующая нерелевантная
проверка, honest unchecked, изменённый claim со старым evidence. Наблюдённый результат
относится к выбранному случаю и use, не ко всем спекам.

### 5.5. Целевая секция

About system; use_claim, concept_claim, main_uncertainty — условная локальная адаптация
SYSE.2, не предписанная DPF схема. Целевые claims/checks допустимы. Описание концепции
без checkable claims и нужного validation basis остаётся описанием даже в specs.

### 5.6. Условные расширения

Machine и генерация тестов/typestate отложены в инженерном варианте: дублирующая модель
должна оправдать стоимость живым использованием. Это рекомендация, не закрытый автором
продуктовый выбор. Unknown fields сохраняют существующий материал, но не превращают
его автоматически в исполняемую модель.

### 5.7. Связь с реализацией

Имя **implemented_by** выбрано Иваном. Форма ниже — инженерный вариант для Ф1/Ф3:
claim-local список `{ref, covers}`, где ref обозначает file/dir/sym, а covers называет
реализуемую часть и границы. Модуль/класс выбирается через поддерживаемый точный
селектор; новый универсальный тип адреса не выдумывается ради подписи «модуль».
Отсутствие связи допустимо и видно; оно не доказывает отсутствия реализации.

Несколько реализаций одного claim и несколько claims одного символа допустимы.
Указание реализации не утверждает её корректность, приёмку или покрытие тестом.
Правка местоположения не меняет смысл поведенческого claim; снятый старый run не
становится свежим evidence из-за обновления locator. Поля checks и implemented_by
не выводятся автоматически друг из друга или из constrains.

Tree-sitter даёт синтаксические anchors. Resolver различает exact/ambiguous/unresolved/
unsupported и сохраняет основу разрешения. Найденный агентом кандидат — предложение
связи, а не установленная семантика. Runtime-границы — RUNTIME-CONTRACT §7.
Примеры и развитие исполнимых проверок — SPEC-CODE-VERIFICATION-PROPOSAL.

### 5.8. Change carrier — B1 implementation contract

This bounded engineering choice closes the previously open serialization for B1.
It does not approve a public release or the legacy-active policy in §10.
`docs/v10/carriers.md` is a generated exact copy of CONTRACT, not a separately
editable schema. `scripts/v10/sync-contract.py` copies or checks it explicitly.

A change is intent and preparation, separate from the six record kinds. Its sole
writable form is Markdown frontmatter `format: haft.change/1` at
`.haft/changes/<id>.md`. IDs are `chg-YYYYMMDD-8hex`. Every edit (including rebase
or archive) creates a new ID and pins the previous change in `supersedes`.
`change_key` is the first change ID, preserved by successors. There is no writable
latest/head pointer: the reader derives leaves and reports competing leaves.
A change snapshot uses the same byte-preserving snapshot envelope as a record.

```yaml
format: haft.change/1
id: chg-20260923-00112233
change_key: chg-20260923-00112233
title: Preserve order total when cancelling
intent: Change cancellation while preserving the agreed accounting property
state: open                      # open | archived; neither means accepted/passed
created_at: 2026-09-23T10:00:00Z
supersedes: []
rationale: []                     # refs to decisions/notes; no duplicate chosen/why
tasks:
  - id: test-cancel
    text: Run the cancellation property on the candidate
    done: false
    results: []                   # explicit record/report refs; checkbox is not evidence
checks: []                        # checking selectors, not run results
evidence: []                      # exact evidence refs
patches:
  - base: spec-20260923-aabbccdd@sha256:<64hex>
    operations:
      - op: MODIFIED
        claim_id: total-preserved
        claim:
          id: total-preserved
          kind: law
          text: Successful cancellation preserves total for new and paid orders.
          unchecked: Property oracle is still to be implemented.
        reason: Clarify the supported input states
```

Each section patch names its exact authored base; ordinary patches never mutate
subject/about. New sections are created through `remember` as proposed specs,
then addressed by the same patch mechanism. A docs-only change may have empty
patches with a nonempty `no_spec_change_reason`. A section's `claims` may be empty
while authored; validation must report absent validation basis, not a checked spec.
The body of a change is explanatory prose, preserved byte-for-byte on an unchanged
round trip, never a second imperative patch language.

Operations are ordered within a section (explicit authored intent), independent
between different bases. Repeating a section base in one package is invalid.

| Operation | Exact rule | Conflict or explicit loss boundary |
|---|---|---|
| ADDED | Full claim with unused ID | Identical existing claim is a no-op; different content or retired ID conflicts |
| MODIFIED | Full replacement of named existing claim, retaining its ID | Omitted unknown fields are preserved; removed examples require explicit IDs in `remove_examples` plus reason; supplied empty/replacement examples cannot silently drop old IDs |
| REMOVED | Remove named existing claim, with nonempty reason | ID is appended to `retired_claim_ids`; dangling local dependencies conflict; old snapshots remain |
| RENAMED | Existing `claim_id` to unused `new_id`, reason required | Preserve all content and rewrite exact local claim refs; tombstone old ID; external refs remain on their original editions |

An example has `id` and optional given/when/then/text; ID is stable in its claim.
An omitted `examples` field on MODIFIED preserves existing scenarios. Intentional
removal names exact example IDs. `remove_fields` may name explicit unknown claim
fields for removal with a reason; ordinary omission preserves them. A changed
claim text does not inherit old evidence, even when ID is retained. RENAMED is
address repair, not a claim of semantic equivalence established by the kernel.
Ordinary patches preserve the section body, fields and unknown extensions.
Claim JSON and its nested binding, example and evidence-input objects use the
same inline field namespace as their carrier YAML. A complete recalled claim
can be copied into JSON/YAML change frontmatter, including unknown fields, and
those fields can be edited at the same location. `extra` is not a wrapper in
these authoring objects; an explicitly authored field named `extra` remains
ordinary extension data. Older pre-repair JSON responses with an `extra`
envelope must be re-read or explicitly converted before authoring. Other record
JSON envelopes and historical source-snapshot encoding retain their existing
representation; stored carrier and snapshot bytes are not rewritten.
The corrected authoring JSON changes derived preview and typed-revision payload
digests when extensions are present. Re-preview and use a new request ID for a
new edit; this does not normalize or rewrite a previous request receipt.
Replacing body requires `body` plus `expected_body_digest` and `body_change_reason`;
the preview includes the complete before/after body and explicit removed material.
Removing the last claim requires `retire_reason`; an empty section successor keeps
its body and has `retirement: {reason: ...}`. Retirement is visible in projection,
not deletion, acceptance or archive. Adding a claim to a retired section requires
a new section/lineage, not reuse of its retired IDs.

Preview is pure: input change + exact captured base documents → proposed output
content, losses, conflicts and deterministic preview digest. It allocates no IDs,
times or files. Output metadata is supplied at publication. Preview never mutates input/project status, origin or confirmation. Its proposed
output explicitly clears inherited confirmation and shows proposed/agent_proposal. Changed spec outputs default to proposed
agent proposals, with exact supersedes and a reason. An explicit trusted caller
request may publish accepted successors only with `origin: operator_request` and
`operator_confirmed: true`, and the complete replaced proposed/active head set.
The API preserves this caller assertion; it does not manufacture an approval receipt.

`apply`/`sync` are the same task-level publication effect (sync is an alias, not a
second merge). They require change ref, preview digest, request ID and expected
publication generation. They compare authored bases against current selected heads
before staging. An exact proposed authored base is compared with the current
proposed leaves of its lineage; the governing active head remains unchanged.
Multiple proposed leaves are an explicit conflict, never a timestamp choice. If
that proposal has been accepted or replaced, its old edition is stale. An active
authored base is compared with governing active heads. `stale_authored_base` requires explicit rebase; a moved generation
after preview is `concurrent_write`; a repeated request is `replayed`; a reused key
with different payload is `request_conflict`. These results remain distinct.
`rebase` creates a change successor from explicitly supplied new exact bases and
resolved operations; it never retargets old patches automatically. The new preview
must pass on the new bases. Old intent and rejected/conflicting versions remain readable.

Publishing stages every output and snapshot under one transaction manifest in
`.haft/transactions/<transaction-id>/`. The manifest lists request/payload digest,
expected generation, exact input refs and complete output path/byte digests.
Before publication, every output and its snapshot is materialized and references
are validated against the joint prospective projection. A claim introduced by
one output can therefore be referenced by another output of that same change.
Head, generation and lifecycle admission still use the captured pre-publication
state; the prospective projection does not grant acceptance or bypass conflicts.
An invalid package publishes no partial subset or journal.
Snapshots are published before carriers using no-clobber writes. A final immutable
commit marker is the cooperative-reader visibility point. Readers encountering an
unfinished journal report `publication_pending` and do not project a partial batch.
Recovery under the writer lock completes exact already-staged bytes only when
published paths and untouched input bases still match. Any foreign mismatch yields
`recovery_conflict`, retaining all material; recovery never overwrites it. A journal
is tracked durable publication metadata, not a disposable cache or another writable
copy of claims. A request that lost its reply can recover and replay this result.
This protocol does not promise atomicity for an arbitrary external filesystem reader.
A same-request retry whose published carrier bytes have changed externally
returns `replay_conflict` without replacing them. CLI reports a nonzero exit and
MCP reports `isError: true`, retaining the structured conflict and diagnostics.

Archive creates an archived change successor and preserves intent, tasks, refs and
patches. It does not apply patches, accept specs, retire a section, or infer check
success. Unsynced/unfinished content is reported as warnings. Reopening an archived
change creates an open successor, preserving history. A repeated request uses the
same receipt rule; concurrent heads require explicit selection/resolution.

### 5.9. B1 application boundary and extension seams

The initial B1 CLI/MCP contract used `haft.api/1` with the result
envelope below. The public delivery successor is §5.10 (`haft.api/2`); canonical
formats and the domain semantics in this section are unchanged. Initial envelope: `format`, `operation`, `result_kind`, `data`, `diagnostics`, `basis`,
`coverage`, and explicit `limits`. Application operations: remember, recall, context,
impact, check, change (create/preview/apply/sync/rebase/archive), source
(status/search/inspect). `migrate` is CLI-only staging, sharing the normal writer.
Remember accepts carrier bytes, request_id and exact expected target digests/head
sets; metadata clocks/IDs are provided by the effect layer, not the pure model.
Read operations never infer success or absence from an unreadable scope.
Recall resolves a live alias or record ID before deriving the object's outgoing
relations. The returned exact address identifies those same bytes. Backlinks
match both exact targets and live addresses that resolve to that edition in the
captured view, while retaining the authored target in each edge. Pinned old
evidence remains attached to its historical edition; a moved alias does not
retarget it. A retained historical snapshot can supply its outgoing relations
even after its old canonical carrier is absent.
Optional `context`/`impact` code captures contain the exact file bytes, explicit
build configuration and verified raw index basis. A supplied prior capture is
reconstructed before advisory formatting/dependency comparison; it is not an
implementation proof. `check prepare` returns a declared claim/check contract and
exact uncached runner command without executing it. `check observe` retains the
actual observation classification separately from current-basis sameness/change/
unknown; an old pass remains historical observation, not a new current pass.
Preparation also returns `basis_capture` (`haft.check-basis-capture/1`): exact
implementation/dependency JSON preimage bytes, their file-digest maps and build
configuration, plus the oracle path, half-open byte span and raw digest. This is
an inspectable capture, not evidence that a run happened. The caller verifies
actual file bytes and environment before and after execution. The Go adapter
captures the selected local test-build composition separately from syntactic
navigation: ordinary package inputs, internal and external test variants in the
selected directory, and their local imports. Supported assembly, local headers
and system-object inputs belong to this basis. An unsupported or incomplete
composition refuses exact preparation instead of hashing a known partial subset.
This adapter supports Go 1.25.x, explicit GOOS/GOARCH, ordinary build tags, and
one captured module without workspace, replacement or vendor resolution. It pins
CGO_ENABLED=0, GOENV=off, GOEXPERIMENT empty, GOFIPS140=off and the target's Go 1.25
architecture-feature default in both the declared basis and run environment.
Custom compiler tool tags, compiler-feature-tag source selection, cgo/foreign
sources and embed inputs are unsupported. Assembly includes must be captured
literal same-directory files or named Go toolchain/generated headers. Unknown,
parent, nested or macro includes cannot establish an exact local basis. External
module and toolchain implementation bytes remain declared inputs outside local
inspection; arbitrary runtime inputs are outside this adapter's claim.
Observation attribution checks the declared oracle reference, package/test
selector and supported command identity as well as the captured basis. The
test selector must identify one unambiguous selectable top-level Test declaration;
methods, TestMain and duplicate internal/external registrations are not exact oracles.
A known contradiction yields `unattributable`, unknown currentness and no declared-check
binding, while preserving the supplied run's raw observation. Code/oracle/input
drift alone retains a legitimate historical result with changed current basis.
The Go adapter accepts only its bounded command form with the exact package, uncached selection
and declared build tags; unknown, duplicated or contradictory flags are
unattributable rather than attributed to a different configuration.

Repository identity is an explicit stable token in optional `.haft/project.yaml`
(`format: haft.project/1`, `repository_id`). Missing identity means local root scope,
not a globally unique identity. Cross-repository writes/references/stores are not B1.
Optional method configuration is namespaced under `method` and remains opaque B1
preparation guidance; it cannot change carrier validity or the write root. Settings
are separate from memory and from operator authority. B1 has no user schema editor.

Check adapter input records exact claim target, selector, code/check basis,
conditions, optional seed, observed runner output and exit status. Its result is
one of passed, assertion_failure, skipped, not_run, environment_failure,
unattributable. Unknown cause stays unattributable. A zero-test exit 0 is not_run.
The agent/reviewer must assess oracle fit; the adapter cannot establish it from a
name or a call edge. Evidence uses retain scope and exact targets regardless of
adapter verdict; no run or adapter result automatically accepts a spec.

### 5.10. B1 progressive delivery baseline — haft.api/2

This is the selected implementation of the operator-requested B1 progressive
reading. It changes delivery, not carrier/history, authority or evidence meaning.

## Versions and entrypoints

Public CLI/MCP requests and results use `haft.api/2`. `haft.api/1` is the initial
B1 wire version and remains the internal domain result representation; public
v1 requests receive a bounded unsupported_format diagnostic naming v2. No stored
carrier, snapshot, change, receipt or report is rewritten. Canonical carrier
formats remain unchanged. The application has a common delivery entrypoint for
CLI and MCP; adapters do not construct alternative views.

Existing operations keep their domain arguments. Added read controls: `view`
(`summary`, default; `detail`; `bytes`), `part`, `cursor`, `expected_digest`.
`limit` (0..500) caps member/directory pages; the byte budget can return fewer.
Memory discovery retains the complete captured selection for further pages.
Nonzero legacy `offset` is rejected; use the returned cursor.
`operation: read` accepts an exact persisted record/claim `ref`, or a returned
`result:sha256:<64hex>` transient result reference. It never accepts filesystem
paths. The next request includes format/operation/ref/view/part and applicable
basis preconditions; clients need not calculate offsets or decode cursors.

## Result and pure presentation

The envelope retains operation, result_kind, diagnostics, basis, coverage and
limits, and adds is_error and delivery. `is_error` is computed from the complete
domain result before presentation and controls both CLI exit and MCP isError.
Delivery errors (stale, expired, missing, corrupt, invalid cursor/part) are errors.
`data` is the selected projection, not the former duplicated full result.
The bounded basis projection prioritizes stable identifiers, including memory
generation when available. It indicates the count of basis members omitted or
shortened in that projection; this is presentation loss, not loss of captured
facts. When an exact result ref is available, the returned parts directory and
`basis` part provide the executable route to the complete recorded basis.

`delivery` names view, part, lifetime, digest, total_bytes, encoding, offset,
returned_bytes, complete, available parts/count, omissions and next_request.
Completeness concerns this selected representation/page only, not domain coverage,
claim satisfaction, current basis or reading the whole artifact. Excerpts carry
explicit incompleteness. A part directory is itself paged. An absent continuation
has a reason. Small detail objects are returned whole; large objects/arrays expose
member pages with exact child read requests. Member paths are server-issued names
with stable numeric child positions, not client-authored arbitrary JSON pointers.

Named parts include result, diagnostics, limits, basis, record, claim/claims,
carrier, body, snapshot, links, backlinks, evidence_uses, expected, basis_capture,
observation, code_capture, preview, source_body and source_snapshot when present.
Fenced JSON report blocks in Markdown are separately addressable authored data,
with labels and exact body spans; parsing one never certifies its claims. A
summary does not infer a verdict from prose. It distinguishes application outcome
from supplied runner outcome, current_basis and declared_check_binding. Historical
evidence displays its recorded claim, uses and basis with currentness unknown
unless actually compared. Diagnostics and source provenance remain discoverable.

Search returns descriptors and labelled excerpts, never raw carriers. Result,
relation and directory pagination binds the complete selection, query/filter
inputs and captured memory/code/source basis. Changed generation returns stale.
Record content/snapshot parts read by exact persisted ref survive head changes
and process/cache restart. Relations are still current-generation selections.

## Byte delivery and budget

The maximum serialized MCP tools/call result is 8192 bytes, including text,
structuredContent, isError and JSON escaping. The pure presenter measures that
exact envelope and fits content before serialization; no serialized JSON slicing.
CLI returns the identical application object. Text-only clients decode the text
as the same object as structuredContent. JSON-RPC correlation envelope is separate;
IDs are bounded and protocol diagnostics are bounded with explicit omissions.

`bytes` reads exact carrier/snapshot bytes or deterministic JSON bytes of a named
structured part. Each response includes full SHA-256, total_bytes, zero-based byte
offset, returned_bytes and executable continuation. Encoding is base64 for bytes;
text detail uses utf-8 chunks ending on code-point boundaries. The digest covers
the complete named member bytes, not the containing edition. The exact edition
remains in ref/basis. The last chunk has complete=true and no continuation.
All chunks together restore the full bytes and digest, including unknown fields.
Small complete structured claim detail is authoring input; summary/excerpt is not.
A larger claim can be reconstructed from bytes or its complete member tree.

Cursors are opaque versioned values binding selector identity, view, member digest,
selection basis and next offset. Parameter changes, malformed cursors and out-of-range
offsets fail explicitly. A changed source/selection never silently mixes pages.

## Transient results and retention

Large check prepare/observe, context captures, previews, source results and errors
use a content-addressed disposable result cache beneath .haft/.cache/disclosure.
This is not a canonical carrier, historical edition or new storage engine. The
result ref identifies exact captured input/result bytes. Cache loss gives expired;
invalid digest gives corrupt. Process restart alone does not remove it. Cache
publication failure leaves the original outcome/receipt visible and explicitly
reports continuation unavailable; no fabricated durable ref. Cache IO rejects
symlinks and addresses only digest-named entries in its confined root.

Mutable selections, preparations, context and previews recheck their memory and
code basis before continuation; source results recheck source-tree basis. A
changed basis yields stale and asks to repeat the original query. Observe and
write receipts retain their captured outcome; reading them does not recompute
currentness. Full inputs/outputs remain captured while cache is available.

An explicit remember may supply `retain: [{ref: <result ref>, part: <part>}]` to
append exact captured bytes/digests to the authored evidence body server-side.
It still requires a complete ordinary carrier, scope and target relation; retention
never invents evidence meaning. This avoids transporting large outputs through
model context. Original request identity governs replay; a committed retry does
not depend on a surviving transient cache. Unknown retained extensions survive.
New retained attachments identify `haft.retained-part/1`, media and exact byte
digest. Recall exposes each verified attachment as a named `report_N_content`
part alongside its unchanged envelope: JSON members and UTF-8 text remain
selectively readable without decoding a full base64 field in model context.
Standalone retention of a capture stdout/stderr stream is allowed only when
that stream is complete. An incomplete prefix, including a pipe that stopped
draining before the byte cap, is rejected with an executable request to retain
the capture's `result` part instead. That part carries the incompleteness and
full observed byte counts and digests. Complete streams retain normally,
including when only the other stream overflowed. Existing retained
attachments and readers are unchanged.
An invalid attachment digest is reported in its directory entry; the original
envelope remains readable and no decoded part is advertised. Decoding is a
representation of supplied data, never an attribution or attestation claim. A cache entry is limited to 128 MiB and
the delivery cache to 512 MiB; quota or publication failure is explicit and does
not change a committed receipt. No automatic eviction or durable-history claim
is made. YAML values outside JSON (such as .nan) expose labelled YAML parts and
exact canonical bytes instead of a lossy JSON authoring object. An authored
claim selector longer than 512 bytes uses a bounded transient selector; the
original exact ref and canonical bytes remain in the full result. Repeating
recall on that original ref reconstructs delivery after cache loss.
For shorter exact selectors, the application first measures the complete reply
with the persisted ref. If its required read routes cannot fit, that read page
receives a disposable selector as well. A continuation cursor is verified
against the original request before its offset is rebound to that selector.
The original pinned ref remains directly readable after cache loss, while the
returned disposable route reports its actual lifetime.
An immutable part of that captured exact edition remains readable after an
unrelated memory generation change; mutable directories and relations still
recheck the captured generation.

## Discovery and examples

Initialize instructions, tool and input descriptions describe summary, incomplete
views, part directory, next_request, stale/expired and authoring. Generated skills
use the same shared description. No host-specific tool name, shell, shared files,
Python or output-limit setting is needed to read. Applicability requires the full
governing source body, not its summary.

First read: {"format":"haft.api/2","operation":"recall","ref":"spec:order-cancel#admission"}.
The response supplies exact_ref and read requests for claim/body/relations.
A returned request, for example
{"format":"haft.api/2","operation":"read","ref":"<exact-ref>","view":"detail","part":"claim","expected_digest":"sha256:<member>"},
reads the full selected claim when it fits; otherwise its member page has child
requests. Copy next_request verbatim for continuation. Bulk clients choose
view=bytes and verify the complete digest. Diagnostic readers choose diagnostics
or a report member directly, without downloading snapshots.

### 5.11. T01A task-tool contract — one application API

This section is the selected T01A implementation contract. The observed B1
interface in §5.10 and [INTERFACE-BASELINE](INTERFACE-BASELINE.md) remains a
historical baseline. T01A and T01AR technical checks support only the behaviors
they exercise; native host tool selection and comparative usefulness remain
separate T03 observations.

The default MCP profile advertises exactly five tools. Their names group the
same `haft.api/2` application operations; they do not create five domain APIs.
Each tool argument still contains `format: "haft.api/2"` and `operation`.
The catalog narrows `operation` to that tool's entries. A non-default branch
requires `action`; omission is allowed only where the existing application
operation already has a defined default. The selected operation and action
determine allowed fields, required fields, canonical application effect and
result description.

| Advertised tool | Existing application operations and action branches | Effect boundary in T01A |
|---|---|---|
| `haft_read` | `recall` (including `legacy`), `context`, `impact`, `read` | Reads records, code context, bounded parts and continuations; no canonical publication |
| `haft_write` | `remember` (including `terms` and retained result refs) | Publishes carrier/terms through the shared writer |
| `haft_change` | `change` (`create`, `list`, `show`, `preview`, `apply`, `sync`, `archive`, `reopen`, `rebase`, `update`) and `recover` | Mixed reads and explicit writes; recovery may complete a prior publication |
| `haft_check` | `check` (`structural`, `prepare`, `observe`) | B1 structural/runner-result work; prepare does not run tests. Observe accepts the same optional `code_config` used by prepare for currentness. T01 adds an explicit bounded capture action separately |
| `haft_fpf` | `fpf` (`status`, `search`, `inspect`) and compatible `source` spelling | Reads the pinned FPF/DPF source; fetch/pin mutation is outside this tool |

The CLI keeps `haft10 api --input FILE|-` and its `haft.api/2` request/result envelope.
It can execute a returned `next_request` or named-part request without changing
its JSON arguments. A B1 Codex installation can refresh exact generated assets
with `haft10 init --codex`; edited generated assets remain a preflight conflict.
`haft10 serve --profile legacy` advertises only the compatible single `haft`
tool. Its schema is generated from the same closed action catalog: previously
tolerated unsupported action/field combinations are rejected before an effect,
while valid B1 requests retain their application meaning. The default profile does not advertise `haft`
alongside the five tools. Both MCP profiles and CLI call the same application
core. Grouping cannot alter a committed `request_id` receipt, payload digest,
pinned edition or historical bytes. A retry with the same logical write request
has the same idempotency/conflict meaning under either profile.

One catalog is the source for tool names, operation/action branches, allowed and
required arguments, canonical publication and environment effects, descriptions, examples, generated skills and
schema checks. Every tool's schema is closed at the action branch, and the
server validates the same branch before any effect. Unknown controlling fields,
unsupported actions and wrong-tool operations fail without invoking the core
effect. Authored carrier bytes and defined inline content extensions retain
their exact preservation rules from §§1.3, 2 and 5.8; a closed control schema
does not erase unknown authored content. An explicit JSON `null` for an optional
nil-able request field has B1 omission semantics, including `snapshots`; required
`null` and scalar `null` are rejected. This does not permit a field on the wrong
operation/action branch. Retrying a valid B1 logical write preserves its
transaction receipt, payload digest and historical bytes as a compatibility
requirement. T01AR's frozen B1 replay observes one note only; its terms,
change and retain checks exercise new candidate writes, not historical B1
fixtures for those operations.

MCP annotations aggregate the strongest possible effect of each advertised
tool on its environment, separately from canonical application publication.
Even a canonical read can create the stable `.haft/.runtime/writer.lock` and a
disposable `.haft/.cache/disclosure` entry for progressive delivery. Thus all
five default tools and the legacy tool have `readOnlyHint=false`; only tools
that can publish or change canonical project data use
`destructiveHint=true`. Cache and lock creation alone are additive, not
destructive publication. `idempotentHint=false` is conservative because the
same arguments can capture a changed basis or add a new transient result.
`openWorldHint=false` reflects the pinned local project/source boundary.
Fresh read paths that open the store require a writable `.haft/.runtime` lock
location. Progressive transient continuations also require writable
`.haft/.cache/disclosure`; cache publication failure is reported as unavailable
continuation and does not change a committed receipt. A read-only project root
therefore cannot be advertised as reliably read-only execution. Hints are
host metadata, never authority or proof that an action is safe.

The B1 delivery views, named parts, stable member selection, cursor/basis checks,
`is_error` classification and exact serialized MCP result limit of 8192 bytes
still apply. A returned `next_request` or named-part read request remains an
ordinary `haft.api/2` request whose `operation` is `read`; the enclosing
`delivery.read_tool` identifies the advertised MCP tool that can execute it:
`haft_read` in the default profile, `haft` in legacy. The client calls that
tool with the supplied request arguments verbatim. Server instructions,
tool descriptions and result text must explain summary → named part →
continuation and stale/expired outcomes without depending on installed skills
or host-specific tool search. When present, the read-tool binding counts toward
the 8192-byte result budget; serialized errors share that bound. No B2B
workflow, shared-root or source-sync operation is introduced by T01A.

### 5.12. T01 bounded capture of one declared project test

`check` gains the explicit `capture` action in the shared application API and
the `haft_check` task tool. The legacy `haft` profile and `haft10 api --input`
route the same action through the same catalog and application boundary. A
capture request names one exact claim (`ref`), its declared `check_ref`, an
explicit human-readable `scope`, a stable `request_id`, optional `code_config`,
and bounded `capture` options containing `timeout_ms` and `max_output_bytes`.
The latter is one aggregate stored-byte limit across stdout and stderr, not a
separate allowance for each stream.
These controls are required or optional only on their declared action branch;
wrong-branch fields fail before execution. Neither a schema nor model-generated
request text grants execution authority: the task must explicitly authorize
running this declared test within its scope. The selected command is exposed by
prepare and capture rather than supplied or guessed by the caller. The
`capture` action is the only check branch that starts a project process.

The application prepares the declared Go test from the exact current claim,
oracle and local code basis. Its one uncached JSON command, exact test selector,
project-root working directory and pinned Go environment are fixed by that
preparation and reported with the result. The caller does not supply alternate
shell text, command flags, cwd, environment or runner bytes. The optional
`code_config` selects the same supported build configuration as prepare and
observe. Unsupported or incomplete basis, an unresolved/ambiguous selector, or
a changed basis before the process starts fails closed. Preparation, structural
checking, observation of an external run, read and continuation never start a
test. The existing `observe` input remains valid for an externally executed
runner; capture does not turn an imported observation into an executed one.
Prepare and capture expose the same pinned Go environment, including
`GOPROXY=off` and `GOSUMDB=off`, and the same selected build configuration.
External observe remains usable with its stated historical attribution limit.
The runner inherits host PATH/HOME/temp locations; their contents, executable
bytes and module cache are not captured by the local code basis. The checked
Go version and pinned Go variables narrow this dependency but do not establish
a hermetic build or reproducible host environment.

If the terminal receipt/reply cannot fit their measured serialized capacity,
capture refuses before claiming the request ID or starting the test with a
distinct capacity diagnostic. The caller can use `check/prepare`, obtain
separate authority for a bounded execution of its exact command, then submit
the independently captured runner observation through `check/observe`. This
alternative does not turn preparation or observation into execution authority.
Valid long historical refs remain readable; no fixed claim-length ceiling is
inferred from one measured capacity example.

Capture records actual process-start outcome, exit status or signal/timeout,
stdout and stderr bytes, the selected command, selector, cwd, pinned environment,
limits and exact pre-execution claim/code/check/dependency basis. It drains
process streams in chunks and stores at most that aggregate byte bound while
recording byte counts and digests for all bytes actually drained. On a
completely drained stream these cover its full output; if a pipe stops
draining early, they cover only the observed prefix and the capture is
explicitly incomplete. Output beyond the stored byte bound is also incomplete
and cannot establish
`passed`. The process verdict still uses the check adapter's distinctions:
assertion failure requires the explicit reviewed failure contract and matcher;
wrong selector, skipped and zero-test results do not become passes; start/build/
environment failure and timeout retain their causes. The actual observation
classification and current-basis same/changed/unknown remain separate. A
post-run change cannot promote an older pass to a current pass. The adapter
does not decide whether an oracle truly covers the claim or whether any
evidence should be accepted.
If stream setup fails before process start, capture retains the known no-start
fact and OS diagnostic as an environment failure. Empty observed streams have
zero byte counts and the digests of empty bytes. No EOF or output-completeness
claim is inferred for pipes that were never established.
On darwin/linux, the runner finishes cleanup of its ordinary owned process
group on normal exit, timeout/cancel and incomplete-pipe exits before the
post-run basis comparison. A pipe that stops draining early is incomplete
capture even when the byte cap was not reached. Deliberately detached or
external processes are outside this group boundary.

The capture result and its full available output are stored server-side under
one disposable result ref. Summary names outcome, basis, limitations, byte
counts/digests and executable read/retain continuations. Named output parts
remain readable in bounded chunks with full-member digest and completeness;
retaining `[{ref, part}]` attaches the exact captured bytes through the normal
`remember` writer without a model-authored base64 copy. A result ref alone is
not durable evidence. The existing complete carrier, exact target, scope and
receipt rules still govern publication; no capture automatically calls
`remember` or approves a specification.
If cache publication fails after execution, the first bounded reply still
reports the known outcome, exact basis and process accounting, with explicit
`continuation_unavailable` and no fabricated result ref. Where receipt IO
works, bounded terminal facts preserve those facts for a same-ID retry without
retaining full output. If receipt IO also fails, the reply separates the known
in-memory outcome from persistence uncertainty. This is a safety receipt, not
a durable evidence carrier or task journal.
Transient publication and terminal receipt publication each receive their own
finite attempt. Exhausting the cache attempt cannot consume the receipt's
attempt; caller cancellation and genuine receipt IO failure remain explicit.
The bounded reply names `receipt_persistence` as `confirmed` or `uncertain`
when this distinction is material.

Before process execution, the server claims the `request_id` (1–512 UTF-8
bytes, no control characters or surrounding whitespace) and exact logical
request in noncanonical `.haft/.capture-receipts`, separate from the disposable
result cache and excluded from canonical memory generations. Invalid ID shape
is caller validation, not infrastructure unavailability. A matching retry
returns the captured outcome and available ref without executing again; a
different request under the same ID conflicts. If a crash leaves execution
outcome unknown, the retry reports pending/unknown and does not launch another
process. Pending can also mean the original process is still running: an
identical retry later can recover completion. A new ID is a deliberate new
execution after inspecting the original, never the default recovery step.
Process restart alone does not invalidate an available captured result. If its
disposable bytes are lost, expired or corrupt, the response retains any
available bounded terminal facts and gives an explicit fresh-capture choice
with a new ID; it never silently reruns the original request. A continuation
reads only the stored capture and never starts the test again. This is bounded
retry for the test effect, not a scheduler or durable evidence journal.
Every completed same-ID retry explicitly identifies itself as replay. It keeps
the original observation and immediately-after-run basis unchanged, and adds
a separate non-executing current-basis assessment (`same`, `changed` or
`unknown`) made at retry time. Changed or unknown current basis cannot be
presented as a fresh current pass. An exact result-ref read remains a historical
snapshot and does not perform that retry assessment. The retry summary names
`execution_replay=true`, the immutable
`recorded_post_run_basis`, and `current_basis_assessment`; its presented
`current_basis` is the retry assessment. The stored result and observation
parts retain their original bytes and basis.
For historical receipts without the newer expected-contract digest, replay
compares supported expected contracts by one canonical semantic form, rather
than the old map encoding against the current struct encoding. Unsupported or
underdetermined historical contracts yield `unknown`; actual code, claim,
generation or pinned Go environment differences can still yield `changed`.
Non-effectful
recall/context/prepare/observe may be repeated to reconstruct a lost
disposable result when their inputs remain available; capture expiry or
corruption never asks a client to repeat the effectful original.
The no-rerun guarantee relies on the safety receipt remaining present;
deleting `.haft/.capture-receipts` resets that local retry history.

The shared catalog classifies `check/capture` as a project-process environment
effect without canonical project publication. `haft_check` keeps
`readOnlyHint=false` and `idempotentHint=false`: lock/cache writes and this
explicit test effect are possible. It advertises `destructiveHint=true`
conservatively because project test code can change its environment even when
Haft publishes no canonical carrier. CLI, both MCP profiles and generated guidance
must expose the same action, controls, retry and effect distinction. Their
complete serialized MCP result, including the read-tool binding, remains
bounded by 8192 bytes. This section defines the selected T01 behavior;
technical acceptance requires E2.02 process fixtures and exact candidate
evidence rather than this text alone.

## 5.13. T01B: versioned spec content and selected reauthoring

`format: haft/2` is a **record carrier format only for `kind: spec`**. A new spec
declares it explicitly; an omitted format retains the historical `haft/1`
default for compatible callers. The new form keeps the same `id`, `title`,
`about`, `slug`, `aliases`, `subject`, `terms`, `receiving_use`, `claims`,
`constrains`, `links`, `sources`, body, exact edition, `implemented_by`, `checks`,
L/A/D/E claim kinds, and inline unknown fields as §1 and §5 define them. Example:

```yaml
format: haft/2
id: spec-20260925-00112233
kind: spec
title: Cancellation total
about: domain:Billing.OrderCancellation
slug: order-cancel
origin: agent_edit
created_at: 2026-09-25T10:00:00Z
receiving_use: Check total preservation before changing Cancel
claims:
  - id: L1
    kind: law
    text: Cancel preserves the order total
    checks:
      - {ref: test:TestCancelTotal, covers: paid and new orders}
```

`status`, `operator_confirmed` and `legacy_status` are **not fields of a v2
spec**, even when supplied as `false` or an empty scalar. Its `origin` names the
edit provenance (`agent_edit` or `operator_edit`), never acceptance of a
product choice. The writer defaults an omitted v2 origin to `agent_edit` and
does not infer an operator edit from a commit, review, caller-supplied flag,
or model text. Until a trusted operator-edit route exists, the task-level
writer refuses caller-authored `operator_edit`; the reader still understands
its provenance when encountered in valid external/historical bytes. Decision
records keep the `haft/1` `proposed/active` and
`operator_confirmed` boundary; `haft/2` decisions are unsupported. A direct
operator choice remains necessary to publish an active binding decision.

An admitted v2 spec is projected as **`current` content**, selected by its
exact edition and explicit supersession. `current` never aliases `active`,
`accepted`, `implemented`, `passed`, or release authority. Another current
head in the same lineage yields an explicit content conflict; neither file
order nor timestamp chooses one. Context/impact present current spec content,
active or contested binding decisions, implementation locators, and evidence
uses as distinct rows and relations. A successful content write does not
clear a binding-decision conflict or make its applicability certain. An
unresolved `implemented_by` target is a link diagnostic, not proof that the
implementation is absent; a declared `check` is not a run. Claim evidence
retains its exact target edition and never moves to a successor by claim ID.
Projection resolves explicit supersession from validated exact predecessor
relations independently of carrier enumeration order. A linear A→B→C chain
has C as its head in every file order; a true branch remains a conflict. An
active v1 head and a current v2 head in one lineage are a mixed-format
conflict with exact participants in projection, context and structural check.
Neither format wins by order, status, or recency. Repair of that mixed branch
requires a separate explicit authority/content choice; this ticket supplies
diagnosis, not a merge, acceptance, or general repair effect.

`remember` may create an explicit v2 spec with no status/confirmation input.
For subsequent v2 claim edits, the existing `haft.change/1` preview/apply
patches keep their exact-base, loss accounting, CAS, and receipt behavior,
but materialize a v2 current-content successor without status or confirmation
metadata. A caller cannot use patch metadata to smuggle an `active` v2 spec.
Legacy v1 patch output and acceptance rules remain historical B1 behavior.

Readers interpret `haft/1` **only under §1–§5's historical rules**: active,
proposed, migrated/historical, and their exact snapshots retain the same bytes
and state. An old proposed spec does not become current on read, index rebuild,
or ordinary migration. An unknown `haft/N`, unsupported kind/format pair,
or malformed v2 state field is preserved for diagnostics and exact reading but
excluded from the semantic projection. A version label alone establishes no
FPF specification use; E.10.D2:4.1.3 still needs checkable claims and a
capable named validation relation for the stated receiving use.

The **only v1→v2 conversion** in T01B is one selected spec through
`haft.api/2` `change/reauthor_preview` and `change/reauthor_apply` (the same
actions in both MCP profiles). Preview requires `ref` naming one exact pinned
`haft/1` spec edition. It is read-only and deterministic: it verifies the
predecessor bytes, its live selected head and competing heads; reports the
old format/state/origin and complete new content prototype, exact preserved
claim IDs/extensions/body/sources and predecessor ref, the content/authority/
implementation/evidence distinction, material loss/diff (if any), a
`preview_digest`, and the captured `memory_generation`. It allocates no ID,
time, receipt or file. A migrated/historical v1 carrier is eligible only when
the selected exact edition is present and its reauthoring does not guess an
unresolved live head. Branch ambiguity returns a conflict for explicit repair,
not a winner. In particular, selecting a proposed v1 branch while a distinct
active branch governs cannot silently retire that active branch.
Preview also names every pending v1 proposal in the affected lineage by exact
ref. It states that converting the selected head leaves those proposals
unchanged and that each needs a separately authored v2 change if it should
inform current content. This list and consequence participate in the preview
digest and remain available through bounded disclosure. The decision band
uses the same candidate policy in context and preview: active or contested
`choose_now` decisions with the same `about` or overlapping declared scope
selectors are advisory candidates for assessment, not automatically
applicable governing decisions. A band with no candidates is explicitly
unassessed rather than proof that no binding decision exists. Frontmatter
re-encoding, including comments and YAML lexical form, is reported as a
deterministic normalization note distinct from semantic `material_losses`;
the predecessor's exact bytes and snapshot remain available.

T01BR2 narrows the conversion promise: a valid historical carrier may remain
readable yet contain an extension value whose YAML tag or type cannot survive
the selected v1→v2 encoding. Preview must refuse that conversion before
publication, naming each discovered exact extension path and tag/type; apply
must refuse the same basis before any write. Custom local tags and meaningful
standard tags such as `!!binary` are not silently converted to untagged
strings with `material_losses: []`. Comments, whitespace and lexical spelling
of an otherwise preserved scalar remain disclosed normalization rather than
semantic loss. Ordinary scalar and container extensions continue to convert.
The original carrier, snapshot and historical refs remain readable and exact;
this refusal does not retroactively invalidate them. The same preservation
rule applies at a shared write boundary only where that boundary would
otherwise publish the same lossy conversion.

T01BR3 applies one source-to-proposed-publication preservation test to every
affected authored YAML effect: `remember`, selected reauthor preview/apply,
change create/revise, and change preview/apply/sync. A ready preview or a
durable write cannot silently change retained extension meaning in a supplied
carrier, an exact base, or supplied claim/scenario content. The comparison uses
source YAML scalar tag, type and exact value before a typed decoder rounds a
number or drops a tag. It rejects unsupported explicit tags (including binary
values), implicit numeric rounding and float-to-integer conversion with exact
readable paths and loss diagnostics before publication; it does not suppress
unknown fields to make the comparison pass. Harmless spelling, comments,
whitespace and container formatting remain normalization when their resolved
meaning is preserved. Old carriers, journals, receipts and exact references
remain readable; this rule does not retroactively reject historical reads or
invent an approval route for a lossy write.

The selected operation identifies intentional changes narrowly. Reauthor may
rewrite only its specified format, identity, status, origin, supersession and
receipt/provenance fields; an explicit v1 `status: !!str proposed` is eligible
because status is intentionally absent from the v2 result. Change operations
use stable claim and example IDs when comparing retained content, so explicit
removal, rename, reordering and authored replacement do not become positional
false losses. A requested removal remains visible in the ordinary change-loss
accounting and cannot waive retention of unrelated extensions. Publication
still uses exact basis/CAS and the existing receipt/replay integrity rules;
identical replay names its original output, while missing or corrupt published
bytes remain a replay conflict. Final public responses remain bounded to 8192
bytes in both profiles, with readable exact paths and a route to the original
source rather than a direction to edit history to bypass refusal.

T01BR4 refines that preservation test by the declared carrier schema. A known
string field retains the exact string consumed by its reader even when YAML
implicitly labels an unquoted date, timestamp or number; an explicit custom or
meaningful non-string tag does not gain a blanket waiver. An optional known
field may disappear from normalized output only for a schema-equivalent absent
value, such as its supported empty, null or false form. The equivalence is
specific to the record kind, field and nested schema; unknown fields and
extensions, including extensions inside known containers, retain their raw
YAML tag, type and exact value. This applies to remember, reauthor, change
input, pinned bases and revision successors. A supported timestamp spelling
or positive-infinity spelling is compared by its exact YAML scalar type and
resolved value; large numbers and integral float types are not rounded through
float64 or reclassified as integers to make a comparison pass.

An exemption belongs to the actual requested effect. Archive and reopen do
not treat supplied but ignored intent, tasks or patches as authored revisions;
those retained values remain guarded. Update and rebase may exempt only fields
they actually apply. For a MODIFIED claim, a same-ref check, implementation
binding or evidence input retains its unrelated extension meaning when a
declared field such as covers or applicability changes. Unique existing refs
can establish list-item correspondence across reorder; ambiguous refs do not
authorize silent loss. Actual requested removals and replacements retain the
ordinary change-loss accounting and do not waive unrelated retained data.
Preview, publication, original bytes, receipts, CAS and exact read/replay
boundaries remain as above. Generated skill and both MCP profiles' guidance
describe this guard as applying to remember and change across record kinds,
not just to spec writers.

T01BR5 binds revision exemptions to the applied successor and retained item
correspondence, not to the presence of a supplied field. A task's stable ID
keeps its unchanged extensions guarded when another task field or the change
intent is edited. Omitted extensions of an otherwise unchanged same-ID task
are retained; a changed task with no supplied extension map remains an explicit
whole-item replacement. Patch
base and operation context identify unchanged nested meaning through update or
explicit rebase. A changed exact base is an authored address edit, not a waiver
for unchanged claim bindings. When a retained item's correspondence is not
established, revision refuses that affected value with a precise path before
publication. Actual item/field removal or replacement remains available.

For claim and example extension maps, only authored changed leaves are exempt;
retained siblings keep their raw YAML tag, type and exact value. Sequence-valued
extensions may replace an unambiguously addressed element, append or remove a
known end segment, while retained positions remain guarded. An ambiguous move
does not become a conversion-loss waiver. Duplicate binding refs remain valid
content: plain declared-field edits, additions and explicit list removal or
replacement are not refused solely for the duplicate. Retained extensions,
interpretation basis and raw tagged or exact-number meaning require established
item correspondence or a precise ambiguity refusal. These rules apply to
both haft/1 and haft/2 authored bases and both MCP profiles, with existing
preview/CAS/replay and bounded continuation behavior.

Apply requires the **same exact `ref`**, `preview_digest`, `request_id`, and
`expected_generation`. It recomputes preview under the writer's current
capture and rejects a changed edition/head set, changed preview, or moved
generation before publication. The successor receives a new record ID,
`format: haft/2`, `origin: agent_edit`, a `write_receipt`, exact pinned
`supersedes: [<selected-ref>]`, and a readable reauthor reason. Other semantic
fields and body come from the selected v1 bytes, without caller-supplied
replacement content. The predecessor file and snapshot stay byte-exact and
historical refs keep resolving their prior interpretation. Its v1
status/origin/legacy provenance remain inspectable in that pinned snapshot;
the v2 edit origin describes only the new operation. Publication uses the
existing durable transaction/receipt path; a same-request retry replays the
same result and different payload conflicts. Apply and identical replay expose
the exact published successor ref as `published_successor_ref` and a usable
`exact_read_request` from the durable receipt. The full result may retain its
historical `new_ref` alias, but generated guidance and the bounded summary
name the two fields actually available there. The ref identifies the edition
published by that request; it does not assert that the edition remains a live
head after later writes.
An interrupted or error reply does not promise those fields: an identical
`request_id` retry recovers them only after the committed output and its bytes
pass integrity checks. A missing or changed published output is
`replay_conflict`, not a reconstructed success.
The result distinguishes published content from decision authority,
implementation and evidence, and fits the 8192-byte MCP response limit. This
is a selected, reversible content edition transition, not a batch rewrite,
acceptance of the old proposal, or a live migration.

## 6. Свидетельство

Один evidence сохраняет наблюдение/результат; uses — единственная каноническая форма
применений. CLI может нормализовать короткие флаги, но не хранит второй корневой формат.

```yaml
kind: evidence
about: domain:Billing.OrderCancellation
claim: total сохранился в 10000 сгенерированных случаях new и paid
observed_at: 2026-09-23T10:00:00Z
method: property-based проверка
source: <доступный отчёт или вывод с точным locator>
basis:
  kind: code                       # code | data | report | config | interview | period | other
  ref: <точная проверенная сборка или состояние кода>
  conditions: <существенные параметры среды и входов>
uses:
  - id: u1
    target: <spec-id>@sha256:<64hex>#L1
    check: <проверка и её точная редакция в basis или сохранённом отчёте>
    polarity: supports             # supports | weakens | inconclusive
    scope: прямой вызов Cancel для new и paid
    disposition: pass              # optional bounded use
    receiving_use: проверка этой реализации Cancel
    window: ...                    # только при реальном окне
    reopen_when: меняется Cancel, генератор или трактовка total
```

При создании ядро разрешает live target и закрепляет существующий snapshot. Source не
ограничивается командой: команда описывает повторение, а состоявшийся результат требует
восстанавливаемого вывода/отчёта/наблюдения. Basis выбирается наблюдением, не URI предмета:
интервью о dir остаётся interview; production-результат может требовать config и интервал.

Для прогона кода ref различает проверенный build и commit исходников; dirty tree не
обозначается чистым HEAD. Сохраняется commit + digest/снимок существенных изменений либо
идентичность сборки с основанием соответствия. Неизвестная нужная редакция ограничивает
use и не заполняется выдуманным SHA.

Polarity — локальное огрубление, не обязательный enum A.2.4. Точный исходный verdict и
ограничения сохраняются, если огрубление их скрывает. Inconclusive не равен доказанному
отсутствию эффекта. Disposition A.10 относится к одному receiving use, не всему
наблюдению; отсутствие допустимо. Реальные scope/windows/dates не теряются. Age-only
не выводит автоматическую непригодность.

Цель — точный record или claim. Если источник поддерживает только часть записи, нельзя
заменить её whole-record target: точная часть сохраняется либо связь остаётся узкой
неразрешённой с ограничением. Миграция не создаёт фиктивную историческую цель через
digest свежеперенесённой записи (§9).

## 7. Локальные отношения

Это небольшой реестр Haft для context/impact, не автоматическая FPF RelationSignature.
Ссылки утверждают связь; наличие ссылки не устанавливает истину, полномочие или
результат. Направление каждого вида задано отдельно.

B1 canonical `links` members are `{kind, target, reason?, legacy_type?}`;
kind is `relies_on` or `relates_to`. A premise needs an explicit reason. Other
relations below use their named fields, not duplicate link entries. Unknown
fields remain data; unsupported relation kinds have diagnostics and do not gain
stronger meaning by endpoint shape.

### 7.1. constrains

Из decision choose_now/spec к file/dir/sym: «при изменении этой области учитываются
данные claims с их статусом и применимостью». Файл — селектор, не участник Work.
Пример: гарантия доставки для ingestion. Контрпример: файл лишь менялся при реализации.

Proposed constrains не действует. Constrains dir — объявленная область ограничения;
about dir — тематическая релевантность. A.6.6:4.7 иллюстрирует различение, но не
определяет локальный predicate вместо Haft.

### 7.2. relies_on

Из любого сохраняемого вида с claim/выбором к основанию: «этот claim или выбор использует
указанное содержание как предпосылку». Изменение даёт кандидата на review, не вывод о
неверности зависимого. Причина и используемое содержание восстановимы из записи;
«решает эту проблему» само по себе не классифицируется как premise.

Адрес live либо pinned по receiving use; для options выбора — pinned. Навигация,
provenance и реализационный след не превращаются сюда по типам концов. Неуверенная
старая связь остаётся relates_to/legacy с отчётным ослаблением.

### 7.3. Evidence use

Из evidence к target в uses (§6), со своими polarity/scope/use. Id use устойчив внутри
точной evidence edition; адрес `<evidence-id>@sha256:<64hex>#u1`. Его хранение не
доказывает выполнения проверки или достаточности опоры.

### 7.4. supersedes

Из преемника к точным predecessor snapshots (§3). Не дублируется в links/superseded_by.

### 7.5. relates_to

Навигация от записи к записи/предмету/файлу без premise или constraint. В impact:
linked/navigation, не mentioned. Legacy_type сохраняется; потерянное отличие
implements/refines/... не объявляется сохранённой семантикой.

### 7.6. sources

Цитирование FPF/DPF сохраняет `ref` паттерна либо документа и один `source_revision`:

- `{kind: git, repository: <identity>, commit: <полный git SHA>}`;
- `{kind: local_tree, repository: <identity>, tree_digest: sha256:<64hex>}`;
- `{kind: unknown, reason: <почему редакция не восстановлена>}`.

Для восстановленного источника дополнительно сохраняются publication_path, полный
body_digest, диапазон lines и snapshot_ref на точные прочитанные байты. Git HEAD
родительского проекта не выдаётся за FPF commit. Dirty local FPF остаётся local_tree,
а не официальной публикацией. Реальная provenance ответа чтения не заменяется свежим
HEAD. При unknown сохраняются доступные исходные locators без выдуманных SHA или bytes.

Минимальный source snapshot также хранится в `.haft/editions/sha256/<64hex>.json`, с
`format: haft.source-snapshot/1`, точными source bytes в base64 и provenance. Его digest
считается по полным сохранённым bytes как в §2.1. Сохраняется именно прочитанное тело и
его границы: excerpt не объявляется полным паттерном. Referenced source snapshots
переносятся вместе с носителями; недоступность bytes даёт unresolved, не live fallback.
Это переносимость нужного источника, не копия всего FPF на каждую запись.

Изменение тела даёт кандидата на перечитывание; цитата не доказывает применение паттерна
или результат. Правила чтения Git/local_tree и фиксации одной сессии — RUNTIME-CONTRACT.

### 7.7. Остальные значимые поля

About, claim refs, terms, checks, implemented_by, evidence inputs участвуют в проекциях явно, даже если
не лежат в links. Одна сериализация на концепт; индекс может иметь обратные рёбра без
второго writable источника истины. Unresolved не подменяется похожим объектом.

### 7.8. implemented_by

Из claim спеки к селектору реализации: «здесь расположена реализация указанной части
этого утверждения» (§5.7). Обратная навигация вычисляется индексом; отдельное writable
`implements` на стороне кода не создаёт второй источник истины. Нормативное ограничение
при правке, ссылка на тест и наблюдённый результат остаются отдельными отношениями.
Контрпример: файл просто упомянут в обсуждении или тест вызывает функцию — этого
недостаточно для данной связи.

## 8. Context и impact

### 8.1. Context

Ответ на file/symbol содержит рамку поиска, index_generation, coverage
complete/degraded/unavailable с причинами и отдельные conflict/currentness diagnostics.
Complete касается только объявленной рамки и прочитанных источников, не всех ограничений
мира. Частичная недоступность даёт degraded; пустота при unavailable не означает
отсутствия ограничений.

Полосы: constrained (принятые записи с объявленной областью, совпавшей с запросом), proposed, related,
evidence. Конфликтующие головы явно contested, не один непротиворечивый governing set.
Точная кодовая привязка отделяется от legacy-совпадения.
В related типизированно показывается implementation traversal к claims; она не
попадает в constrained только из-за implemented_by. Проверки и evidence доступны
от тех же claims с собственными scope и основаниями.
Совпадение области — кандидат для чтения, не установленная смысловая применимость:
агент проверяет условия claims и текущую работу.

Возвращаются релевантные evidence uses, включая отрицательные, спорные и разные scopes.
Временная сортировка помогает навигации, но последнее evidence не отменяет остальные.
Лимит ответа не может молча скрыть refutes за новым supports: нужны видимое противоречие,
continuation и ограничения coverage. Необработанные uses не считаются проверенными.

Сигналы target snapshot changed, code/check changed и basis unknown разделены. Hash-diff,
возраст и неизменность файла не являются полной смысловой оценкой актуальности.

### 8.2. Impact

Linked с подтипом constraint/implementation/premise/evidence/succession/navigation/source; mentioned для
текстового совпадения; unresolved для неразрешимой связи. Видны направления, исходные
утверждения, рамка поиска и поколение. По claim/record/file/source восстанавливается
путь к кандидату на review. Не выводятся автоматически «затронуто», «не затронуто»,
«перепринято» или «нарушено».

## 9. Перенос из 9.x

Единственная подробная карта — [MIGRATION-MAP.md](MIGRATION-MAP.md). Количества в ней —
снимок одной проверки, не вечные константы экспорта. Объём поддержки и допустимые потери —
[MIGRATION-POLICY-2026-09-23.md](MIGRATION-POLICY-2026-09-23.md). Команда инвентаризирует
фактический доступный вход: у обнаруженных основных записей есть отчётная судьба,
исключённые метаданные можно показать группой, непроверенная область обозначается явно.
Полное понимание каждой старой внутренней схемы не является условием выпуска v10.

`haft migrate --from-9x --dry-run` показывает входы, открытые выборы, отображение и
потери без остановки процессов, записи в live `.haft` и коммита. Staging и отчёт в явно
выбранном output-каталоге вне live `.haft` допустимы; dry-run не переключает проект.
Реальный переход после
разрешённых условий: согласованная остановка только писателей проекта, read-only
исходная DB, отдельный staging, отчёт и один ограниченный коммит перехода. Не убиваются
чужие процессы и не включается посторонний WIP; конфликты обнаруживаются до изменения
рабочего дерева. Исходная DB остаётся нетронутой.

По принятым ответам Ивана MethodRun/RefreshReport/WorkCommission не переносятся и не
требуют нового архива; отдельного comparator нет. После уточнения рамки известные
пропуски, включая decision/note/spec, не являются безусловным запретом cutover:
пользователь видит их и выбирает подготовленную часть либо восстановление нужного.
Отчёт различает точный перенос, ослабление, needs_rewrite, прежнюю неразрешённость,
непрочитанную область и намеренно непереносимые виды с explicit field_loss/record_loss.
Пропуск в v10 не означает уничтожение оригинала. Повреждение результата операции,
незащищённые источники, активные старые writers и перезапись новых данных блокируют
переключение до устранения. Недостающее содержание ограничивает опирающееся на него
использование, но не требует переоценить всю историю перед началом работы в v10.

Агент помогает восстановить выбранные записи как proposed с происхождением. Штатная
команда переносит известное механически; смысловую актуальность нельзя вывести из
успешной конверсии. Промпт и руководство квалифицируются на кандидате, а не обещаются
безотказными заранее. Чистый старт — явный вариант с сохранённым доступом к v9.

Перенос не добавляет operator_confirmed, не доказывает прежнюю authority из active,
не превращает footprint в constrains, не заменяет unknown historical edition свежим
digest. Claim_refs/scope сохраняются; поддержка части записи не расширяется до whole
record. Несовместимый материал остаётся доступной историей с ограниченной проекцией,
а не вымышленным новым смыслом.

Применение исторических active-решений как действующих ограничений v10 — открытый выбор
§10. До него экспорт и историческое чтение можно реализовать/проверить; принятие этой
политики не объявляется состоявшимся.

## 10. Исторические открытые выборы и границы baseline

Этот раздел сохраняет границу исходного B1 черновика. Последующее прямое поручение
Ивана выбрать решения для B2 выполнено в D1–D6; оно закрывает указанные там design
choices для реализации, а не публичный release или автоматическую активацию
унаследованных решений. Исторически этот документ не закрывал вопросы 3/5/7/8 журнала. Он предлагает
согласованный вариант для обратимой реализации в изоляции и пробы. Операторские ответы
фиксируются отдельно, без авторского пересмотра принятых ответов; последующее прямое
уточнение Ивана о допустимых потерях отражено в §9 и MIGRATION-POLICY.
Выбор имени implemented_by и цель OpenSpec coverage закрыты прямым запросом; форма
change carrier и остальных новых интерфейсов остаётся инженерным предложением (§5.8).

| Вопрос | Инженерная рекомендация | Что не установлено |
|---|---|---|
| Отношения, claims/harness, evidence, условный options | Минимальные формы выше; наблюдать полезное действие следующего агента | Принятие публичных форм и польза на реальных задачах |
| Исторические active-решения | Сохранить статус/контент без выдуманного подтверждения; отдельно выбрать проекцию | Считать ли прежнее local active применимой нормой v10; authority не доказана одним status |
| Claude PreToolUse hook | Явный opt-in до пробы | Default не выбирается автоматически по одному пропуску context |
| Индекс | B1: in-memory projection и JSON transient cache; источники/editions в Git | Новый backend не выбран; SQLite не требуется без measured need, см. D4 |
| Machine и генерация | Отложить до живого receiving use | Рекомендация, не навсегда запрещённая возможность |

Автономный исполнитель выполняет разрешённую обратимую реализацию и проверку черновика.
Новая пользовательская норма, активация неоднозначной импортной authority, публичный
breaking cut-over и релиз не выводятся из готовности схемы или успешного check.


## 11. Выбранные deltas B2A по отдельным tickets

[D1–D6](B2-DESIGN-DECISIONS.md) определяют целевые изменения в своей области.
T01B вводит `haft/2`: spec authoring без status/operator_confirmed; decision
binding сохраняется. Current spec content, authority, implementation и evidence
не смешиваются. Reader `haft/1` и старые interpretation/snapshots сохраняют
прежний смысл. Только адресное preview/CAS переавторство выбранной spec создаёт
новую версию; массового перехода и silent activation нет.

T01A задаёт tool grouping по §5.11, не меняя semantic core. Его фактическая
реализация устанавливается кодом и E2.01, а не этим контрактом. T01 добавляет
bounded test capture, не новый автономный runner. T01C убирает лишнюю ручную
metadata и измеренное дублирование payloads; история, refs, exact bytes и
failure distinctions не удаляются ради короткой schema. Детальные new-format
examples/validation замыкает соответствующий ticket перед writer и сопровождает
regression tests.

T02 переносит только проектную установку host: повторный `haft10 init --codex`
в скопированном или перемещённом проекте адресует текущий корень и явно
указанные текущие executable/source locations. Абсолютные пути в managed host
config являются адресами установки, а не идентичностью, редакцией или authority
FPF/spec/project records. Перенос не меняет закреплённый source pin, содержание
носителей, выбранный default/legacy MCP profile или историю проекта. Только
доказуемо сгенерированные Haft части могут обновляться; точная граница ownership,
конфликты и эффекты заданы [RUNTIME-CONTRACT §8](RUNTIME-CONTRACT.md).
При смене физического root прежний source внутри прежнего root не наследуется
молча: требуется явный `--source-root`, который выбирает вызывающий. Тождество
доступных корней устанавливается по фактической идентичности каталогов, при
недоступном прежнем корне применяется ограниченное сравнение записанных путей.
Наследуемый внешний source через старый symlink сохраняет адрес того же
существующего физического target; новая публикация из копии не выбирается
автоматически. При невозможности установить target нужен явный выбор.
`--source-repository` наследуется по точным правилам RUNTIME-CONTRACT §8.1.
Отказ сначала даёт короткую законченную инструкцию повторить init для выбранного
проекта с `--root` и `--source-root` и причину, затем необязательные адресные
детали в bounded ответе. Новые owned файлы создаются с запрошенным mode `0600`
с учётом process umask; замена сохраняет mode существующего файла, в том числе
`0644` при umask `077`. Незавершённый init сообщает и ещё не проверенные
релевантные пути, не называя их завершёнными.
