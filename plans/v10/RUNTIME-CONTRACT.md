# Haft v10: контракт исполнения

Дата основы: 23 сентября 2026. Уточнено 24 сентября после B1 review;
контракт T01A — 25 сентября. Baseline B1 отделён ниже от выбранной реализации
T01A и остальных планируемых расширений; документ не заменяет runtime evidence.
[D1–D6](B2-DESIGN-DECISIONS.md) задают новые границы. T01A меняет упаковку tools;
spec authoring и capture остаются задачами T01B и T01 соответственно.
Приложение к [WORKPLAN.md](WORKPLAN.md). Семантика записей, редакций и отношений
определяется [CONTRACT.md](CONTRACT.md); здесь — способы чтения, записи и восстановления.
Виды носителей определяются только CONTRACT. Здесь зафиксированы также требования
к исполнению расширения OpenSpec coverage; его сериализация замыкается в Ф1.
Отдельный daemon или прежние lifecycle-gates не возвращаются.

## 1. Граница ядра

- Чистое ядро получает байты, идентификаторы и явный контекст; возвращает разобранные
  значения, диагностику, проекции, изменения или отказ. Оно не читает часы, Git, файлы,
  окружение, SQLite и сеть. Время и случайный идентификатор поступают извне.
- Разбор формата, редакции, проверка концов отношений, supersede, выборка контекста и
  классификация impact — чистые преобразования. Ни одно не устанавливает истинность
  evidence, полномочия оператора или применимость паттерна FPF.
- Оркестрация соединяет результат ядра с конкретным IO. Эффектный слой выполняет
  чтение, блокировки, публикацию, Git, SQLite и транспорт; он не дублирует семантику.
- MCP и CLI вызывают один task-level API. Исторический B1 рекламировал один MCP
  tool `haft`, 10 operation spellings и action branches; полный список, включая
  пишущий `recover`, — [INTERFACE-BASELINE](INTERFACE-BASELINE.md). Миграция — CLI.
  Контракт T01A задаёт пять task tools в default MCP profile и прежний `haft`
  только в явно выбранном legacy profile. Ни одна упаковка не создаёт второй
  merge или скрытый эффект.
- API различает `found`, `absent`, `unresolved`, `invalid`, `unavailable` и конфликт
  там, где эти состояния меняют действие вызывающего. Ошибка чтения не становится
  `false` или пустым списком. Ограничение/обрезание ответа всегда видимо.

## 2. Переносимая истина

Истина проекта — носители и требуемые ими закреплённые снимки в отслеживаемой Git части
`.haft/`, включая схему интерпретации из CONTRACT. Git переносит эти файлы; наличие
коммита не доказывает семантическую корректность или принятие человеком.

`CONTRACT.md` определяет идентичность редакции и состав закрепляемого содержания.
Runtime обеспечивает следующее:

1. Для закреплённого адреса доступны исходные байты снимка и интерпретационная основа,
   включённая в его редакцию. Новый term-map не переинтерпретирует старый снимок.
2. Снимки живут в отслеживаемой части `.haft/`, отдельно от `.cache/`. История Git,
   локальная SQLite и глобальное зеркало FPF не являются единственной копией снимка.
3. Полный digest проверяется при загрузке. Короткое отображение хэша не используется
   как уникальный ключ. Совпадение адреса при разных байтах — конфликт, не overwrite.
4. На новый clone, в пустом HOME, без старой БД и сети можно восстановить переносимые
   записи и закреплённые цели. Недоступный внешний первоисточник остаётся недоступным;
   сохранённое указание на него не превращается в проверенное свидетельство.
5. Отсутствующий/повреждённый снимок даёт `unresolved` с точным адресом. Подмена его
   текущим текстом запрещена. Неизвестный формат остаётся opaque по CONTRACT.

Индекс и прочие вычислимые данные находятся в `.haft/.cache/` и игнорируются Git.
Временные файлы и runtime-lock также игнорируются. Include-ссылки не делают соседний
репозиторий переносимым автоматически: рамка ответа называет фактически прочитанные
источники и недоступные части. Недоступный include не означает пустую память.

## 3. Запись: один публикуемый носитель

`remember` создаёт новую запись или преемника. Оно не меняет файл предшественника и
не переписывает существующий носитель под тем же ID. Отношение замены и вычисляемые
статусы следуют CONTRACT; конкурирующие преемники не разрешаются по времени записи.

Порядок эффекта:

1. Получить кооперативную блокировку writers проекта с ограниченным ожиданием.
2. Прочитать необходимые цели и их фактические байты; закрепить требуемые редакции.
   Валидировать предложенную запись на этой основе. Смена цели не переносит старую
   поддержку на новую редакцию: live evidence/options targets требуют
   expected_target_digest из предыдущего чтения по CONTRACT §2.3. При несовпадении
   отказать с конфликтом; не закреплять текущие байты вместо наблюдавшихся ранее.
3. Опубликовать отсутствующие неизменяемые снимки раньше ссылающейся записи. Уже
   существующие снимки проверить по полному digest. Сбой здесь оставляет только
   неиспользуемый снимок; автоматическая очистка снимков в v10 не требуется.
4. Записать временный файл в том же filesystem, закрыть после `fsync`; опубликовать
   финальное имя атомарным **no-clobber create**, затем синхронизировать каталог.
   Обычный заменяющий `rename` не выполняет требование no-clobber. Реализация выбирает
   поддерживаемый примитив, например link готового temp в финальное имя; отсутствие
   такого примитива даёт отказ. Занятое имя никогда молча не перезаписывается.
5. Вернуть ID, путь, digest носителя и статус обновления индекса. Если носитель уже
   записан, а индекс недоступен, ответ прямо сообщает `written` и `index_pending`.
   Повторный вызов не должен создавать другую запись из-за ошибки обновления кэша.

Для безопасного повтора task-level writer принимает `request_id`: он сохраняется в
публикуемом носителе как техническая provenance, а не второй источник его содержания.
Вместе с ним хранится digest канонического **запрошенного** payload до серверных ID,
времени и закрепления live refs. Повтор с тем же ID запроса и digest возвращает прежний
результат; другой payload даёт `request_conflict`. Поиск повтора работает и без кэша.
Технические поля остаются частью точных snapshot bytes по CONTRACT; равенство запроса
и равенство снимка — разные проверки. Ручная правка записи с receipt не получает
повторно полномочие или доверие из старого receipt.
Запрос без `request_id` допустим, но его слепой повтор после потери ответа не объявляется
exactly-once. Формат и placement поля согласуются с CONTRACT при реализации carrier.

Блокировка координирует процессы Haft; внешний редактор и Git её не соблюдают.
Поэтому runtime не обещает атомарный снимок всего рабочего дерева или CAS против
произвольного внешнего редактора. Он не перезаписывает его файл; фиксирует прочитанные
редакции и обнаруженные конфликты. Ручные правки допустимы, но проходят тот же разбор
при следующем чтении. Конфликт Git не трактуется как валидный носитель.

### 3.1. Применение пакета изменения

Расширение Ф1/Ф3/Ф10: pure core получает typed patch и точные исходные редакции,
возвращает preview с создаваемыми преемниками, сохранённым содержанием, удалениями и
конфликтами. Это отдельный эффект от записи произвольной заметки. CLI, MCP и agent
skills пользуются одним вычислением; «умное слияние» в тексте skill не заменяет его.

Проверяются две разные основы: редакция, на которой автор готовил изменение, и
состояние во время применения. Смена первой требует явного rebase/conflict resolution;
защита от гонки второй не доказывает пригодности устаревшего намерения.
Идентичный replay идемпотентен. Принудительный overwrite не скрывается за ADDED.
Предупреждение об удаляемом сценарии/неучтённом prose не исчезает в batch-операции.

Multi-carrier apply использует staged outputs, manifest/journal, project writer lock
и восстанавливаемый publish. B1 CONTRACT §5.8 фиксирует commit marker как точку видимости и
publication_pending/recovery_conflict для незавершённой операции. Нельзя назвать несколько rename атомарной транзакцией.
Перед публикацией материализуются все outputs и snapshots пакета. Их ссылки
проверяются на совместной предполагаемой проекции; проверки исходных голов,
generation и жизненного цикла сохраняют прежнюю transaction-current основу.
Новая claim и ссылка на неё в другом output допускаются одним пакетом, а не
требуют промежуточных неатомарных публикаций.
Точный формат записей/patches имеет один источник в CONTRACT, не в этом приложении.
Archive меняет состояние пакета работы; он сам не принимает новую норму и не объявляет
проверки успешными. Batch conflict не разрешается автоматически временной сортировкой.

Shared planning roots и repository references получают явные identity/revision и
read/write scope. Reference не расширяет authority или write roots. Общее дерево
спецификаций и личный список workspace folders — разные объекты. Ни глобальная
транзакция нескольких Git-repo, ни автоматический push/sync не обещаются. Частичная
доступность/применение и способ продолжения показываются адресно.

## 4. Поколение памяти и текущесть ответа

Поколение памяти вычисляется из отсортированного манифеста относительных путей и
полных content digests **прочитанных байтов**, плюс версии правил разбора и значимых
настроек разрешения ссылок. В него входят term-map, переносимые снимки и фактические
include-источники. `.cache`, runtime-lock и незавершённые temp в манифест не входят.

`mtime` и размер — только подсказка для оптимизации. Они не доказательство неизменности:
замена файла равной длины с прежним временем должна обнаруживаться. Первый корректный
вариант читает и хэширует корпус; оптимизация позднее обязана сохранять этот результат.

Перед каждым `recall`, `context`, `impact` и статусным чтением процесс собирает
актуальную основу. Внутри одного результата разбор, отношения и тела берутся из одних
захваченных байтов, не из повторных независимых открытий исходных файлов.

После чтения проверяется стабильность списка и содержимого. При наблюдённой смене —
ограниченный повтор; после исчерпания бюджета — последний целостный снимок с
`degraded`, причиной `source_changed_during_read`, либо `unavailable`, если его нет.
Два одинаковых наблюдения — наблюдённая стабильность, а не доказательство отсутствия
любых промежуточных внешних изменений. Эта граница не скрывается словом «атомарно».

Ответ несёт `basis`: поколение памяти, идентификатор code epoch и состояние её
покрытия, идентификатор снимка FPF/DPF, ограничения/непрочитанные источники. Эти
компоненты фиксируются для запроса; не утверждается одна глобальная транзакция файлов,
кода и внешнего Git. Отсутствие связи при неполном чтении не становится «связей нет».

Recall выводит исходящие связи разрешённого exact endpoint. Входящие связи
сопоставляют pinned targets и живые адреса в текущей захваченной проекции,
сохраняя авторский target. Старый evidence не переезжает вслед за alias.
Конфликт replay после внешнего изменения опубликованных bytes остаётся отказом
и на транспортном уровне: ненулевой CLI exit и MCP `isError: true` при сохранённом
структурированном результате и неизменённом конфликтующем носителе.

## 5. Несколько MCP-процессов и вычислимые данные

Каждый stdio-клиент имеет свой `serve`; отдельный daemon не требуется. B1 читает
canonical files и строит derived in-memory projection заново. Disclosure transient
results сохраняются JSON-файлами в `.haft/.cache/disclosure/`. SQLite index в новом
runtime **не реализован и не обязателен**; SQLite используется legacy migration
reader. Прежний подробный SQLite design здесь снят как неподтверждённая потребность.

Сохраняются независимо от backend:

1. Стабильный advisory writer-lock вне удаляемого `.cache/`, в `.runtime/`;
   bounded ожидание, проверка path identity, один порядок блокировок.
2. Один ответ использует captured bytes/generation. Частичная или нестабильная
   выборка не называется complete, старые links не смешиваются с новыми телами.
3. Canonical carriers, immutable editions и transaction journal не являются cache.
   Commit marker и recoverable publish определены CONTRACT §5.8. Несколько rename
   не называются глобальной атомарной транзакцией.
4. Потеря transient cache даёт `expired` и способ повторного получения/retention,
   не выдуманные bytes. Сохранённое evidence доступно из durable carriers/parts.
5. Cache corruption/path replacement обнаруживаются; retry ограничен, при
   повторном вмешательстве допустим unavailable. Canonical данные не удаляются.

Новый persistent index выбирается только после measured need и отдельного
bounded design. Его schema/concurrency/rebuild не назначаются заранее выбором
SQLite. T01C сначала измеряет реальные payloads и дублирование.

## 6. Источник FPF и DPF

Общий `$HAFT_HOME/fpf` — bare Git-зеркало; чтение идёт по конкретному commit object,
а не из общего mutable checkout. Fetch сериализован отдельным mirror-lock и имеет
timeout. Project lock и mirror-lock не удерживаются одновременно. Успешный fetch не
меняет использованные ранее источники в записях и не принимает решений за проект.

При старте `serve` настроенный ref разрешается в точный commit. Все запросы этого
процесса используют этот снимок до перезапуска; `status` показывает и его, и доступное
обновление. `haft fpf sync` обновляет зеркало, но не подменяет снимок активной сессии.
Проекты с разными pin читают разные immutable objects независимо. Недоступный pin не
заменяется main. Кэш извлечений адресован source identity и версией extractor.

Каждое найденное тело имеет provenance: вид источника, repository identity, commit
либо local-tree digest, путь публикации, полный digest тела и диапазон строк. Readme,
USING-FPF и Suite Reference доступны как документы без выдуманного PatternID.
Сериализация sources/source_revision и переносимых source snapshots определяется
CONTRACT §7.6: git, local_tree или unknown. Вторую форму этих полей runtime не вводит.
Неоднозначный PatternID, отсутствующий конец или неполный источник дают диагностику;
первое совпадение не принимается молча за полное тело.

Локальная `fpf/` имеет объявленный приоритет. Её фактические байты захватываются в
неизменяемый локальный снимок по content manifest; dirty-правки учитываются. Git HEAD
родительского проекта не выдаётся за SHA FPF. Чтения в запросе используют этот снимок,
а не меняющееся дерево. Снимок помечается `local_tree`, а не официальной публикацией.

`remember` сохраняет именно provenance реально использованного источника, переданную
из ответа чтения, и проверяет digest доступных байтов. Оно не добавляет свежий HEAD
вместо отсутствующей provenance. Для закреплённого replay нужные исходные снимки
сохраняются переносимо по CONTRACT; ссылка без доступного снимка явно unresolved.

Офлайн с доступным снимком: чтение работает, `status` сообщает offline и время
последнего успешного fetch. Без снимка: только FPF-функции возвращают unavailable;
обычная память и доступная кодовая информация продолжают работать. При ошибке fetch
прежний проверенный снимок не удаляется. Старый source не объявляется текущим upstream.

## 7. Кодовый индекс и проекции

- Сохранить действующую семантику admission, exclusions, symbol anchors и code epoch.
  Порт переносит нужные свойства, не требует всей прежней машины governance.
- Изменение файла пересчитывает необходимое замыкание зависимостей, в том числе
  reverse imports и пакетную область Go/Python. Требование «ровно один файл» неверно.
- Сравнение incremental результата с чистой полной сборкой на том же fixture —
  критерий корректности. Непричастные файлы не перепарсиваются без причины.
- Параллельные refresh публикуют только законченные эпохи. Context называет использованную
  эпоху, её свежесть, исключения и неполное разрешение; пустой caller list не означает
  отсутствие риска. Exact, module и proximity не смешиваются в доказанную власть.
- Удаление/rename исходника, смена импорта, `.gitignore` и `.haftignore` инвалидируют
  нужную область. Частичный/оборванный анализ не заменяет прошлую полную эпоху без метки.

Claim-local implemented_by разрешается отдельно от constraints и checks. Resolver
возвращает точный anchor, code basis и пределы либо ambiguity/unresolved/unsupported;
первый похожий symbol не подставляется. Смещение строк обновляет координаты. Rename/move
допускает перенос связи только при проверяемом однозначном соответствии; иначе даёт
адресную диагностику. AST similarity и call graph не устанавливают смысл реализации.
Уведомления группируются по claim и нужному действию, не по каждому ребру.

Check adapter описывает selector/oracle, область и существенные условия.
В B1 runner проекта остаётся у agent/IDE/CI: Haft принимает наблюдение.
B2A D3 добавляет ограниченный adapter запуска явно объявленной test command;
он не становится автономным agent runtime. Точная граница T01 задана ниже; её
выполнение требует отдельной E2.02 проверки. Evidence различает passed в указанной области,
assertion failure,
skipped/not-run, runner/environment error и unattributable result. Exit 0 с нулём
выбранных тестов не подтверждает claim. Точная code/check/claim basis, dirty tree,
seed/inputs и среда учитываются настолько, насколько влияют на повторяемость.
Изменение oracle или вспомогательной зависимости требует пересмотра основания даже
при неизменном locator. Structural `check` не выполняет и не сертифицирует harness.

## 8. Init и обновление хостов

Init изменяет только принадлежащие Haft записи конфигурации, сгенерированные skills и
содержимое его маркеров. Иные MCP servers, TOML/JSON keys, инструкции и пользовательские
skills сохраняются. Слияние конфигурации структурное; замена файла целиком недопустима.

Upgrade рассматривает обнаруживаемые global и project-local skills вместе: старый
локальный carrier не должен молча затенять новый глобальный. Exact известный generated
carrier можно обновить; изменённый пользователем или неоднозначный — сохранить и выдать
точную диагностику. `--local` не создаёт глобальные записи. Повторный init идемпотентен.
Перед эффектом повторно сверяются байты; обнаруженная правка отменяет замену этого файла.
Это не CAS против внешнего редактора между проверкой и rename: гарантии кооперативной
блокировки и её ограничение те же, что в разделе 3. При обнаруженной активной внешней
записи init оставляет этот путь нерешённым. Незавершённый init сообщает выполненные и
невыполненные пути; чужие настройки не считаются исправленными по отсутствию ошибки.

`doctor` проверяет фактическую конфигурацию и обнаруженные skills обоих хостов; это
проверка установки, не доказательство их использования агентом. После установки нужны
новые реальные сессии Claude и Codex; старый MCP-процесс не объявляется обновлённым.

Это первые два квалифицируемых хоста. Полная цель из OPENSPEC-PARITY-MATRIX включает
остальные объявленные install targets, profiles/delivery и cloud setup. Реестр
адаптеров различает generated-carrier support, protocol checks и наблюдённый host run;
наличие шаблона не доказывает работу всех IDE. Число skills не ограничивает набор
пользовательских действий. Пользовательские правила/шаблоны остаются отдельно от
kernel semantics; ownership и сохранение чужого применяются к каждому адаптеру.

### 8.1. T02 project-local relocate/re-init (2026-09-25)

`haft10 init --codex` выполняет один project-local preflight и применяет его к
текущему физическому корню проекта. При обращении через обычный filesystem
symlink адрес в managed config указывает на разрешённый корень. CLI и init
используют одну границу эффекта. Старый абсолютный `--root` в скопированном
managed block не заставляет писать в исходный проект. Новый binary и, если
задан, source root берутся только из явных текущих параметров; перенос этих
адресов не означает смену закреплённого FPF source или его содержания.
Если `--source-root` опущен, прежний source наследуется только при неизменном
физическом корне проекта либо когда он расположен вне прежнего корня. Если оба
корня доступны, их тождество устанавливается по фактической идентичности
каталогов (`os.SameFile`), включая разные регистры написания на
case-insensitive filesystem и обычные symlink-адреса. Разные каталоги на
case-sensitive filesystem остаются разными. Если прежний корень недоступен,
используются нормализованные записанные пути как ограниченный fallback;
недоступность сама по себе не доказывает тождество. Вложенность source
определяется по существующему физическому target и границам компонентов, а не
по строковому префиксу. Когда корень изменён, а прежний source равен прежнему
корню или лежит внутри него, init до любой публикации требует явный
`--source-root` и называет прежнюю зависимость. Он не выбирает автоматически
новый `FPF/` из копии и не оставляет молча source из оригинала.

При наследовании существующего внешнего source, записанного старым config
через symlink, init сохраняет адрес именно его разрешённого физического target.
Так скопированный project не зависит от symlink внутри перемещаемого
оригинала. Если эквивалентность прежнего адреса и target установить нельзя,
init требует явный выбор source до записи; недоступный source сообщает
отдельную причину. Явный существующий старый или новый source остаётся выбором
вызывающего. При опущенном `--source-repository` прежнее значение наследуется
независимо от адреса; явное значение его заменяет. Ошибки выбора и
недоступности source отличимы от конфликта владения. В bounded JSON diagnostic
и stderr краткая законченная инструкция повторить init для выбранного проекта
с `--root` и `--source-root` и причина отказа идут перед необязательными
длинными путями; сокращение деталей не скрывает способ восстановления.
Итоговый CLI-ответ не превышает 8192 байт.

Перед любым изменением init проверяет все затронутые пути. Для AGENTS и четырёх
skills владение устанавливается по точным известным generated bytes принятых
версий B1, T01AR и непосредственно предшествующих integrated versions либо по
текущему exact output. Для `.codex/config.toml` прежний block может содержать
старые абсолютные адреса: владение устанавливает точный generated шаблон с
одним `mcp_servers.haft10`, допустимыми адресными полями и одним известным
вариантом `--profile legacy`; другие ключи, аргументы и изменения внутри блока
не принимаются. Адресная правка, которая всё ещё совпадает с этим шаблоном,
сама по себе неотличима от сгенерированного адреса; это предел доказательства
для старого config без отдельного receipt. Если прежний профиль legacy, перенос
сохраняет его; отсутствие `--profile` сохраняет default. Текст вне блоков,
foreign TOML tables/keys, пользовательские settings, skills и данные остаются
побайтно прежними. Изменённый или неизвестный owned candidate даёт
`host_conflict` с конкретным путём до публикации, а не удаление или overwrite.

Каждый затронутый файл публикуется отдельно после повторной сверки его
preflight-байтов и безопасной проверки пути. Обнаруженная конкурирующая правка
останавливает этот путь; повтор после сбоя заново проверяет всё дерево и
допускает уже опубликованные точные generated результаты как no-op. Прерванный
многофайловый init не выдаётся за единую транзакцию: результат или ошибка
показывают `created`/`updated` только для завершённых путей, `unchanged` только
для проверенных неизменённых путей и `unresolved` для каждого релевантного
пути, который ещё не завершён или не проверен, включая ранний отказ preflight
и planned-identical путь до повторной сверки. Наличие одного blocker не
объявляет остальные пути исправленными или неизменёнными; временная публикация не
заменяет пользовательский файл. Не утверждается атомарный CAS против
некооперативного редактора между последней сверкой и filesystem rename.
Новые owned файлы создаются с запрошенным mode `0600` с учётом process umask;
обновление сохраняет mode существующего destination, включая `0644` при umask
`077`, без применения
umask к уже существовавшему mode. Это локальная
политика публикации, без чтения или изменения process-wide umask. Возвращённая
ошибка очищает только созданный этой попыткой temp; остатки после SIGKILL или
потери питания и блокировки на неподдерживаемой платформе не квалифицированы.
Проверка config/CLI доказывает адреса установки, а не использование skills или
tool profile реальной новой сессией хоста.

## 9. Минимальные приёмочные случаи

| Случай | Проверяемый результат |
|---|---|
| Свежий Git clone, пустой HOME, offline, `.cache` отсутствует | Записи и закреплённые редакции восстанавливаются; внешние недоступные источники помечены |
| Два stdio MCP одновременно | Оба работают; запись первого видна второму без restart |
| CLI/ручная правка/Git pull при живом MCP | Новый content generation либо явная деградация, без смешивания тел и связей |
| Изменение равной длины с сохранённым mtime | Новые байты обнаружены |
| Два supersede и Git merge | Оба преемника сохранены, конфликт видим, winner не угадан |
| Сбой до/после публикации carrier; потеря ответа | Нет оборванного canonical файла; retry с request_id не дублирует эффект |
| Сбой чтения/построения derived projection | Явная unavailable/degraded basis; незавершённая выборка не становится complete |
| Удаление transient cache при втором сервере | Нет потери носителей; exact expired/unavailable либо bounded retry; новое чтение восстанавливает вычислимое |
| Ручная смена target/terms после evidence | Старое evidence сохраняет прежнюю редакцию и её интерпретацию |
| Два проекта с разными FPF pin; fetch между inspect и remember | Источники не смешаны; remember сохраняет реально прочитанный снимок |
| Dirty local FPF; duplicate ID; missing End; offline без source | Точные provenance и диагностика, без скрытого fallback |
| Go sibling type / TS reverse import / delete / ignore change | Incremental семантически равен полному rebuild на тех же входах |
| Upgrade global+local; foreign settings; concurrent user edit | Новый generated skill не затенён, чужое не перезаписано |
| Один typed patch через CLI и MCP; agent sync | Совпадают preview/verdict и применённые successors; skill не пишет альтернативный merge |
| Stale authored base; race во время publish; replay | Три разных случая: rebase/conflict, recovery и idempotent result |
| Real property check; zero tests; нерелевантный pass; changed oracle | Наблюдение привязано к точной основе; нет ложного подтверждения |
| Rename/formatting/ambiguous symbol/partial index | Правильная связь либо честная диагностика; отсутствие ложного claim violation |
| Два shared roots, одинаковые paths, missing reference | Явный root/identity/scope, отсутствие implicit authority, qualified partial result |

Каждый случай связывается с одним candidate SHA, командой/сценарием и наблюдённым
результатом. Успех CLI-зеркала не подменяет stdio MCP; отсутствие ошибки сервера не
доказывает применение FPF агентом или полезность продукта. Польза проверяется отдельно
сценариями WORKPLAN, без расширения этого runtime-контракта.

## B1 implementation choices (2026-09-23)

CONTRACT §§5.8–5.9 now close change and API serialization. B1 uses full captured
content scans and an in-memory derived index rebuilt from canonical bytes on each
request. No SQLite cache is necessary for this bounded corpus; §5 states the
backend-independent requirements without preselecting a future database. The stable OS writer
lock outside .cache, content generations, durable transaction journal, bounded
recovery and independent clients remain required. Rebuild may persist a disposable
content-addressed JSON index; it never becomes authoritative. Source retrieval is
pinned local snapshot/lexical in B1; mirror fetch/sync is B2. Existing broader
language/shared-root examples above are roadmap cases, not a B1 scope expansion.

## B1 bounded delivery successor (2026-09-24)

CONTRACT §5.10 defines haft.api/2 views, exact parts and continuations, the
8192-byte serialized MCP-result budget and disposable transient result cache.
Canonical bytes and history remain unchanged. CLI and MCP use one application
delivery entrypoint after domain outcome classification. Protocol metadata and
generated skills expose the same read contract. Qualification is Q01–Q12 in the
B1 disclosure workplan; this contract alone is not host execution evidence.

Explicitly retained result attachments expose verified decoded JSON/text parts
through the same bounded read API after transient cache loss. Exact attachment
bytes and the enclosing historical carrier remain unchanged on read.

## T01A tool and profile boundary (2026-09-25)

The default stdio MCP server advertises `haft_read`, `haft_write`,
`haft_change`, `haft_check` and `haft_fpf`. `haft10 serve --profile legacy`
advertises only the B1 `haft` tool; the server never exposes both catalogs in
one profile by default. `haft10 api --input` keeps the public `haft.api/2`
envelope. The profile is a transport choice, not a project data format or a
separate application service. [CONTRACT §5.11](CONTRACT.md)
sets the operation grouping and delivered read-tool binding.

The catalog supplies operation/action ownership, allowed and required request
fields, canonical publication and environment effect classification, schema, descriptions, examples and generated
skill text. Both profiles validate their advertised schema server-side before
calling the shared application `Service.Call`. A malformed or wrong-tool
request cannot reach a writer, recovery or any other effect. `format` and
`operation` remain explicit `haft.api/2` arguments; action is required for a
branch without an application default. Unknown control fields and unsupported
actions are errors. Optional nil-able JSON controls accept explicit `null` as
omission (including B1 `snapshots:null`); required and scalar nulls still fail.
`check/observe` admits `code_config` so a custom-config prepare is compared
against the same code basis. Defined authored extension fields are data and survive
the same parsing, snapshot and delivery path as B1.

Project-local `init` may replace an exact generated B1 Haft block and its
four exact generated B1 skills with the catalog-based instructions. Text outside
the managed block, host configuration and file modes are preserved. Edited or
unknown generated content remains a preflight conflict, before any file write.

`haft_read` and `haft_fpf` expose no canonical project/source publication. `haft_write`
publishes through the existing transaction/receipt path. `haft_change`
contains both reading and writing branches; `recover` is an explicit writing
operation there, including when it completes a publication after a lost
response. `haft_check` in T01A exposes structural/prepare/observe only and
does not execute project tests. A canonical read can still create the stable
`.haft/.runtime/writer.lock`; computed delivery can add a disposable
`.haft/.cache/disclosure` result. The catalog therefore advertises
`readOnlyHint=false`, `idempotentHint=false`, `openWorldHint=false` for every
tool. `destructiveHint=false` for `haft_read`, `haft_check` and `haft_fpf`:
their possible lock/cache writes do not publish canonical project data.
`haft_write`, `haft_change` and legacy `haft` conservatively advertise
`destructiveHint=true` because their branches can publish or change it.
Fresh store reads require a writable `.haft/.runtime` lock location; transient
continuation requires writable `.haft/.cache/disclosure`. On cache publication
failure, delivery reports the unavailable continuation without changing a
committed receipt. On lock failure, the affected store operation is unavailable.
T01 must reassess the effect model when it introduces bounded test capture.
These host hints grant neither authorization nor evidence of an action's
result. Source fetch or pin change is not a branch of `haft_fpf`.

The application generates bounded result/error data and the same outcome
classification for CLI and both MCP profiles. Delivery names `haft_read` as
the default `delivery.read_tool`; the legacy MCP transport maps that binding
to its sole advertised `haft`. Each returned read/part/continuation request
therefore has an executable tool name in its enclosing result without
rewriting the logical `next_request` arguments, canonical bytes or write
receipts. It must re-fit the complete serialized MCP tool result, including
this binding, text, structured content and `isError`, within 8192 bytes.
Transports cannot truncate JSON or reinterpret a failed operation as success.
An expired transient result, stale mutable selection and exact persisted ref
retain their B1 distinctions. CLI and protocol E2.01 checks establish only
technical behavior; real host tool selection remains T03 evidence.

## T01 bounded capture execution boundary (2026-09-25)

[CONTRACT §5.12](CONTRACT.md) specifies the selected `check/capture` application
operation. It accepts one exact declared claim/check/scope and a required
`request_id`, plus optional `code_config` and mandatory bounded
`capture.timeout_ms`/`capture.max_output_bytes` (one aggregate stdout/stderr
stored-byte limit). The application derives the
same uncached Go test command, exact selector, root cwd and pinned Go environment
as `check/prepare`; the transport cannot substitute shell text, flags, cwd, env
or purported runner output. The authorized task names the declared test and
scope; prepare and capture expose the selected command. A model-supplied field
is not execution authority. Only this
action crosses the project-process effect boundary; structural check, prepare,
external observe and result reads have their prior semantics and never run it.
The runner inherits host PATH/HOME/temp locations; those bytes and module cache
are outside the pinned local basis, so this is not a hermetic environment.
Prepare and capture use the same pinned Go variables, including
`GOPROXY=off` and `GOSUMDB=off`; external observe retains its historical
attribution limit.

The pure check core validates the declared contract and classifies the actual
run supplied by the thin process shell. Application preflight resolves the
claim, selected test and complete local test-build basis, fixes command/env and
rechecks exact inputs before launch. The effect shell starts at most one
process for an admitted new request, without a shell, in the project root with
the pinned environment; it bounds wall time and output storage. It records
whether start succeeded, actual exit/signal/timeout and byte counts and digests
for the bytes actually drained from stdout/stderr. For completely drained
streams these are full-output counts/digests; on an incomplete pipe they are
only observed-prefix accounting. The process streams are drained in chunks even
when aggregate stored output reaches the cap; overflow is explicit and cannot yield
a confident `passed`. A changed post-run basis remains historical observation
with `current_basis=changed` or `unknown`, never an inherited current pass.
Failure to establish stdout/stderr pipes before process start reports a known
no-start environment failure with the OS diagnostic, zero observed byte counts
and empty-byte digests. Pipes that were never established do not gain invented
EOF or output-completeness status.
On darwin/linux, the shell completes cleanup of its ordinary owned process
group after normal exit, cancellation/timeout or incomplete-pipe return,
before the post-run comparison. A pipe that stops draining early is incomplete
even below the byte cap. Deliberately detached or external processes are
outside this group boundary.
The selected test's assertion-failure attribution still requires its reviewed
failure matcher; no generic exit code or output fragment supplies one.
Before claiming the request ID, capture measures whether the bounded terminal
receipt and final serialized reply can fit. A capacity refusal has its own
diagnostic, does not launch a test or consume the ID, and points to the
existing `prepare` -> separately authorized bounded execution -> `observe`
route. The bound is calculated from the actual representation, not a universal
claim-ID length limit; historical exact refs remain readable.

Before launch the shell atomically claims `request_id` with the digest of its
logical request in noncanonical `.haft/.capture-receipts`, separate from the
disposable result cache and excluded from memory generations. The ID is 1–512
UTF-8 bytes without control characters or surrounding whitespace; invalid
shape is caller validation. A completed same-key retry returns the stored
capture without launching and explicitly marks replay. It preserves the
recorded observation and post-run basis, and separately assesses the current
basis without running a process; changed/unknown assessment is not a fresh
current pass. An exact historical result-ref read remains a snapshot. A
different payload conflicts. A pending claim may still be in flight; an
identical retry later can recover completion. A new ID is a deliberate fresh
run after inspecting the original, not default recovery.

Restart with available bytes replays. If transient publication fails after a
known run, the bounded first reply reports its outcome and exact basis with
continuation unavailable and no fabricated ref. When receipt IO works,
bounded terminal facts preserve this known result across restart; if receipt
IO fails, persistence uncertainty is reported separately. The transient cache
attempt and terminal receipt attempt have independent finite budgets. Cache
delay or failure does not spend the receipt's budget; caller cancellation and
actual receipt failure still have honest outcomes. Expired/corrupt
full result bytes retain terminal facts when available and offer a safe
fresh-execution choice. No process runs from a result-ref read, page
continuation or retention. Claims are retained safety metadata and raw outputs
are disposable implementation data, not a canonical evidence record or new
durable task runner. Explicit `remember` with retained ref/part still publishes
through the existing writer and receipt protocol. No shared lock is held merely
to wait for the project test process.
Deleting `.haft/.capture-receipts` resets the local no-rerun history; ordinary
cache eviction alone does not.
Historical expected-contract parts and current contracts are compared through
a canonical semantic representation. If the historical form cannot be fully
interpreted, retry currentness is `unknown`; changed code, claim, generation or
pinned Go environment remains distinguishable. Existing receipt, cache and
part bytes stay unchanged.
Standalone retention of a capture stream is refused when it is an incomplete
prefix or pipe capture; the caller can retain `result` to preserve observed
byte counts, digests and incompleteness. Complete stream retention and
historical attachment readers remain supported.

`haft_check` metadata now reflects a possible project-process effect in
addition to cache/lock creation; it still does not publish canonical Haft
carriers. `readOnlyHint=false` and conservative `destructiveHint=true` reflect
possible test-code filesystem effects; no tool annotation grants authorization.
CLI, default MCP, legacy MCP and
generated skill wording must name capture's effect, fixed command basis,
result ref, bounded reads, retry and expired action consistently. The 8192
byte final serialized MCP limit from B1 remains. E2.02 must exercise actual
test-owned processes, overflow, timeout, lost response/retry and retain
without model-side base64, while separate T03 evidence decides comparative
burden and native usefulness.
The public basis summary preserves useful stable metadata such as memory
generation when it fits, reports omitted or shortened basis members, and gives
an executable exact `basis` part route where a result ref exists. Projection
omission does not imply loss of stored basis; the 8192-byte final envelope and
CLI/default/legacy continuation routing still apply.

## T01B spec edition and reauthor boundary (2026-09-25)

[CONTRACT §5.13](CONTRACT.md) governs `haft/2` spec content. The reader branches
on the carrier format before projecting it: `haft/1` active/proposed/migrated
and their snapshot bytes keep their historical meanings; `haft/2` spec yields
current content without accepted-authority status; unsupported format/kind
pairs stay diagnostic and cannot enter the old projection. Derived indexes may
be discarded and rebuilt without promoting a v1 proposal. Pinned historical
references always use the stored snapshot's format and exact bytes.

Projection resolves validated exact supersession independently of file order:
each linear v2 chain has one final current head and a true branch conflicts.
A mixed active v1/current v2 lineage is a diagnosed conflict with exact head
refs in projection, context and structural check. Neither head wins by format
or recency. No automatic merge or authority choice is available for it.
For a pinned structural check of either mixed-lineage participant, typed
`head_refs` contains the full exact participant set for that lineage, as do
alias and whole-project views of that lineage; a diagnostic excerpt alone is
not the typed participant result. Structural syntax validity remains distinct
from uncontested current content. When a supplied spec alias resolves to this
conflict, the decision candidate band says lineage conflict prevented spec
selection, with `not_assessed` and degraded coverage, rather than claiming no
spec selector was supplied. An empty band does not establish no authority.

`remember` with explicit `format: haft/2` and `kind: spec` and the existing
`haft.change/1` patch effect can publish current content without a spec
status/confirmation. Patches over v1 retain the old proposed/accepted rules.
Decision binding remains a distinct effect requiring a direct operator
request; a spec writer, technical review, check or committed file is not that
request. A successful content publication does not mask a contested binding
decision, and a declared implementation/check link does not become actual
implementation or evidence through projection.

`change/reauthor_preview` reads one exact pinned v1 spec edition and the fresh
head set. It makes no publication; the returned digest binds the complete
conversion result and its losses or conflicts. The bound preview enumerates
pending v1 proposals in the affected lineage and explains that converting
the selected edition leaves those proposals unchanged; incorporating one
requires a separately authored v2 change. It reports deterministic
frontmatter normalization separately from semantic material losses, while
preserving exact predecessor bytes. Its active/contested choose-now decision
band is an advisory candidate set selected by same-about or declared scope
overlap, consistent with context. Empty candidate sets remain unassessed, not
evidence that no binding decision exists. A valid old carrier with a custom
or meaningful standard YAML value tag that the writer cannot preserve stays
readable, but selected conversion refuses before mutation with exact
extension path/tag diagnostics; it is not a ready preview with no material
losses. Harmless lexical normalization remains separately disclosed.
The same source-to-proposed-publication test covers shared `remember` and all
change create/revise/preview/apply/sync normalizers. It compares retained YAML
scalar tag, type and exact pre-decoding value, including numbers too large for
float64 and explicit float values such as `1.0`; unsupported conversion is an
explicit preview conflict or write refusal before durable mutation. Intentional
reauthor control-field rewrites and change deltas are exempt only at their exact
authorized fields, with retained claims/examples matched by stable identity.
Original bytes and refs remain readable. The diagnostic gives the exact path
and recovery by preserving or separately representing the authored meaning;
it offers neither silent coercion nor an approval bypass.

T01BR4 resolves remaining false refusals and exemption leaks without replacing
the source-to-publication comparison. Schema-owned string scalars compare by
the exact string that the existing reader consumes for that field, including
implicit timestamp or numeric-looking YAML spelling. Only schema-owned
optional zero values that decode to the same absence may be omitted; a
path/name match alone does not qualify, and an extension at any depth remains
raw-tag/type/exact-value guarded. Explicit custom tags, large exact numbers
and integral float type changes still refuse. YAML timestamps accepted by the
reader, including space-separated zone-free, lower-case `t` and single-digit
components, compare by their parsed timestamp value; `+.inf` and its normalized
positive-infinity spelling compare as the same float value. This is a bounded
scalar comparison, not a general YAML compiler or float64 approximation.

Revision retention exemptions are calculated from the selected action and
the fields it actually applies. Archive/reopen ignore supplied intent, tasks
and patches for the effect and keep those retained paths guarded; update or
rebase applies and exempts its actual authored fields only. Claim MODIFIED
bindings/evidence inputs use unique same-ref correspondence when available;
only changed declared child fields or explicitly removed/replaced items can
be exempted. Retained child extensions are preserved or a precise ambiguity
or semantic-loss conflict is returned before writing. Ordinary change losses
for actual removals/replacements remain visible. The same rule applies in
preview and apply/sync recomputation under CAS, and never rewrites pinned
predecessor bytes. Both MCP profiles, schemas and generated skills describe
the full remember/change guard and retain exact-path/read recovery under the
8192-byte final response bound.

T01BR5 narrows the same writer guard at applied leaves and items. Update and
rebase compare the saved change to the actual successor by task ID and patch/
operation context; supplying an unchanged task or binding does not exempt its
extension. Omitted extensions of an otherwise unchanged same-ID task are
retained; a changed task without an extension map is a whole-item replacement.
Claim/example extension
maps preserve unchanged leaves, and ordinary sequence edits use only stable
positions, with ambiguous moves refused at the affected path. Duplicate-ref
binding lists permit plain declared-field edits, additions and explicit
removal/replacement when no retained meaning is endangered; retained custom
tags, exact numbers and extensions still require correspondence or a precise
refusal. Old reads remain permissive; preview, apply and sync recompute the
same guard under the existing CAS and 8192-byte disclosure boundary.

`change/reauthor_apply` requires
the same ref, preview digest, request ID and expected memory generation. Under
the stable writer lock it rechecks the exact predecessor/current-head basis,
then stages the new v2 carrier, its snapshot, and the durable receipt through
the existing transaction protocol. Stale content, a competing head, changed
preview, moved generation, request conflict, interrupted publication and
replay remain distinct outcomes. The old carrier and its snapshot are never
rewritten. The new record has a fresh ID and exact supersedes link to its
selected predecessor; the latter's origin and legacy provenance remain
recoverable there. Reauthoring one historical edition is a requested local
write, never an automatic startup/read repair or batch migration. CLI, both
MCP profiles, catalog schema/help and generated skills expose these same
preconditions and effects. Apply and identical replay return the exact
published successor ref in `published_successor_ref` and an executable
`exact_read_request` from the durable receipt, including when later writes
have superseded it. This output reports
published content and does not assert current decision authority,
implementation or evidence. Every final serialized MCP response remains within
8192 bytes, including the read-tool binding.
The guarantee applies to verified successful apply/replay replies, not to an
interrupted or error reply. At a committed-but-reply-interrupted fault, report
the committed effect, reply interruption and explicit identical-request retry
advice without fabricating a returned ref. Same-ID retry verifies the durable
output; deleted or corrupted published bytes remain `replay_conflict` at CLI
and MCP boundaries, never a manifest-only success.
