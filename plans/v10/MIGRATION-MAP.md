# Haft v10: карта переноса 9.x для исполнителя

Дата: 23 сентября 2026. Приложение к [CONTRACT.md](CONTRACT.md) и WorkPlan v3.1.
Это спецификация будущей команды, не свидетельство выполненной миграции.
[DECISIONS-LOG.md](DECISIONS-LOG.md) имеет приоритет с последующим прямым уточнением
[MIGRATION-POLICY](MIGRATION-POLICY-2026-09-23.md) о допустимых потерях. Новые выборы, в том
числе действие исторически active-записей, остаются там, где CONTRACT помечает их открытыми.

## 1. Граница работы

Одна CLI-команда `haft migrate --from-9x [--dry-run]`: согласованный снимок источников,
преобразование в отдельный каталог, отчёт, затем разрешённое переключение и один коммит.
Отдельного `export verify`/сравнителя нет. MethodRun, RefreshReport и WorkCommission не
переносятся и не складываются в новый JSONL-архив; старая база остаётся нетронутой.
Перенос ограничен распознаваемыми входами и не обещает полной совместимости. Известные
пропуски любых видов, включая decision/note/spec, перечисляются с исходным locator и
путём восстановления; сами по себе они не блокируют выбор перехода. Оригинал сохраняется.
Таблицы ниже задают соответствие для распознанных данных, а не обязательство написать
универсальный декодер всех исторических вариантов. Неподдержанный вариант имеет явную
судьбу в отчёте. Сохранение байтов не равнозначно сохранению машинного смысла.

Основание: SYSE.29:4.5 различает прекращение интерфейса и интерпретируемость истории;
SYSE.34:4.1 требует явного соответствия значений и названных потерь. Эти паттерны не
требуют ни старой машины lifecycle, ни отдельного продукта для проверки экспорта.

## 2. Проверенный снимок, не константы мигратора

Источник проверки: `<historical-v9-source>/haft.db`, соединение `mode=ro` с
`PRAGMA query_only=ON`; носители этого checkout. При выполнении CLI пересчитывает всё.

| Источник | Наблюдалось | Судьба |
|---|---:|---|
| artifacts: DecisionRecord | 130 | decision с прежним ID/алиасом |
| artifacts: Note | 191 | note |
| artifacts: ProblemCard | 221 | problem |
| artifacts: SolutionPortfolio | 157 | options с сохранением исходного сравнения |
| MethodRun / RefreshReport / WorkCommission | 850 / 69 / 16 | 935 `not_carried_by_policy`, без новых носителей |
| artifact_links | 670, 27 типов | таблица §5; в том числе 109 связей от исключённых видов |
| evidence_items / evidence | 329 / 10 | 339 evidence; пересечений ID между таблицами нет |
| affected_files | 1178 | 730 у решений, 448 у заметок; §6 |
| affected_symbols | 1748 | 1747 у решений, 1 у заметки; §6 |
| artifact_symbol_bindings | 0 | алгоритм читает таблицу и при ненулевом числе |
| spec_section_editions / baselines | 30 / 28 | содержание секций и исторические сведения; §7 |
| Термины в носителе | 117 | terms.md без приписывания новых значений |

Кандидатов переноса первых четырёх видов — 699. Секции и evidence не входят в эти
699. В БД 26 software- и 4 target-секции; прежний подсчёт 23 software относится к
носителю, не к БД. Сравнивать источники по ID, а не выбирать меньшее число.

`implementation_footprint`: в 23 decision-носителях поле встречается, у 11 оно содержит
files, у 12 — пустой объект. В `artifacts.structured_data` поле есть у 84 решений,
непустые files у тех же 11. Поэтому «23 решения с footprint» не означает 23 непустых
следа реализации. У 7 решений footprint — единственная привязка: 93 affected_files,
из них 76 у 6 active-решений и 17 у одного superseded.

## 3. Чтение, идентичность и сохранение исходного значения

1. Читать таблицы и носители отдельно. DB `structured_data`, тело и carrier могут
   различаться; не вызывать sync/import 9.x как скрытую часть чтения. Для расхождения
   сохранить обе версии с происхождением и вывести `source_conflict`.
2. Старый ID остаётся разрешимым. Ограничение новых ID `prefix-date-8hex` не применяется
   задним числом: уже есть старые короткие и длинные ID. Нельзя переименовывать их без
   таблицы алиасов и переписывания разрешимых ссылок.
3. Сохранять исходные title/content, structured_data, timestamps, kind/status и нужные
   старые поля в `legacy`-данных retained carrier либо в адресуемом приложении к нему.
   Frontmatter содержит только восстановимые новые утверждения; body не считается
   успешно преобразованной связью, claim или источником полномочий.
4. Не заполнять отсутствующий object/question/chosen/why/about/next_use по догадке.
   `no_alternative_reason` не заменяет отсутствующий выбранный вариант. Не назначать
   `system:haft` или `reopen_replay` всем старым записям. Недостаточная семантика остаётся
   `needs_rewrite` с исходником и списком недостающих полей; native-валидность не имитируется.
5. Если реализован недорогой imported-content fallback, он позволяет recall исходника
   без участия в `constrained`, принятии решений и суммировании evidence. Это адаптер
   чтения старого содержания, не новый авторский вид или статус. Если такого пути нет,
   показать непереносимость, locator сохранённого оригинала и возможность переписать
   нужное в v10. Не строить универсальный fallback только ради запрета любых пропусков.
6. UTF-8 проверять до декодирования с заменой. В текущей БД `content` решения
   `dec-20260526-9fdd33ed` содержит некорректную последовательность около byte offset 5798
   (12058 байтов). Для него сохранить исходные bytes в адресуемом byte-preserving
   payload/sidecar, отдельно пометить человекочитаемое декодирование и `encoding_error`.
   Без исходных bytes нельзя объявить decision перенесённым. Это не архив исключённых 935.

## 4. Статусы и цепочки: не создавать новое принятие

`migrated_9x` никогда не получает `operator_confirmed: true` от мигратора. Не вводить
авторский статус `archived`: такого выбора Иван не сделал. Исходный `legacy_status`
сохраняется отдельно от proposed/active и от вычисляемых currentness/retirement cues.

| Старая группа | Число | Безопасная проекция до решения о live-использовании |
|---|---:|---|
| DecisionRecord active | 77 | historical-active; принятие/применимость не доказаны переносом |
| DecisionRecord refresh_due | 1 | historical-active + refresh cue; не убирать сигнал |
| DecisionRecord superseded | 26 | историческое выбытие; связь преемства вычислять только если она есть |
| DecisionRecord deprecated | 26 | historical-retired, без выдуманного преемника |
| Note active / deprecated | 184 / 7 | исходное состояние; deprecated не оживлять |
| ProblemCard active / addressed / deprecated | 194 / 16 / 11 | addressed сохраняет «вопрос закрыт», не означает superseded |
| Spec active / draft | 28 / 2 | historical-active / proposal; без выдуманного approval |
| evidence verdict superseded | 26 | historical-retired evidence, не применять как свежее |

Политика действия исторически active decision/spec — открытый вопрос CONTRACT, а не
право мигратора выбрать active или потребовать массового подтверждения. До его решения
разрешены staging и просмотр результатов; зависимое live-включение этих ограничений
останавливается. Остальные работы плана продолжаются.

25 старых supersedes переносить вместе с исходными endpoints. Если правило продолжения,
статус или цепочка не восстановлены, оставить историческое утверждение в legacy и
диагностику. Не создавать retarget_reason, дополнительный successor или подтверждение
ради прохождения валидатора. Предложенный преемник не выключает принятого предшественника.

## 5. Все 27 типов artifact_links

Ниже `navigation` означает `relates_to` с исходными type/endpoints в legacy и отчёте;
это не утверждение о предпосылке. `weakened` — изменение смысла машинной связи, даже если
её исходное имя сохранено. В реализации dispatch включает source-kind и target-kind.

| Старый тип | Число | Правило |
|---|---:|---|
| based_on | 385 | Детализация ниже; не единое переименование |
| relates_to | 85 | 45 navigation; 40 от MethodRun → not_carried_by_policy |
| refreshes | 69 | Все от RefreshReport → not_carried_by_policy |
| supersedes | 25 | §4; не придумывать недостающее продолжение |
| governs | 19 | Decision→SpecSection; 5 разрешимых TS → navigation/weakened; 14 ES уже unresolved |
| artifact_link | 14 | navigation, исходный тип сохранён |
| ProblemCard | 13 | Note→ProblemCard: navigation |
| problem | 11 | Note→ProblemCard: navigation |
| about | 10 | Note→ProblemCard 8, →Portfolio 2: navigation; не поле EntityOfConcern |
| solution_portfolio | 5 | Note→Portfolio: navigation |
| SolutionPortfolio | 4 | Note→Portfolio: navigation |
| solution | 3 | Note→Portfolio: navigation |
| related | 3 | navigation |
| refines | 3 | navigation/weakened; не сохранять claim «уточняет» как факт |
| implements | 3 | Note→Decision: navigation/weakened; не relies_on |
| clarifies | 3 | navigation/weakened |
| Note | 3 | Note→Note: navigation |
| DecisionRecord | 3 | Note→Decision: navigation |
| weakens | 1 | Note→Decision: navigation с явным исходным weakens; не evidence и не premise |
| supports | 1 | Note→Portfolio: navigation с исходным supports; не evidence |
| supersedes_context | 1 | Note→Portfolio: navigation; не supersedes между видами |
| revisits | 1 | Problem→Decision: navigation/weakened |
| note | 1 | Note→Note: navigation |
| narrows | 1 | Note→Problem: navigation/weakened |
| depends_on | 1 | Note→Problem: navigation/weakened без домысливания предиката |
| decision | 1 | Note→Decision: navigation |
| constrains | 1 | Note→Decision: navigation/weakened; заметка не получает новую authority |

`based_on`: Portfolio→Problem 159; Decision→Problem 121; Decision→Portfolio 92;
Note→Problem 9; Note→Portfolio 2; Problem→Decision 2. Первые три допускают `relies_on`
только когда сохранённое содержание подтверждает смысл объявленной опоры; иначе navigation
с `weakened`. Последние 13 — navigation с исходным типом. 92 decision→portfolio links
ведут к **83 уникальным портфелям**; 74 из 157 не имеют такого входящего решения.
Связь с decision не доказывает намерение replay, а её отсутствие — текущий shortlist.

Уже отсутствующие ES-цели не переназывать TS/SS по похожему тексту. Для unknown type или
неожиданной пары видов действует явный navigation/unsupported branch с исходным значением
и причиной. Нельзя молча пропустить строку, повысить её до ограничения или считать
неразрешённый новый endpoint старым дефектом без проверки исходного snapshot.

## 6. Файлы и символы: переносится роль, а не только путь

Проверять конкретный selector, не наличие binding у решения вообще. В одном решении
могут одновременно быть footprint и governing targets. Если один путь участвует в обеих
ролях, сохранить оба утверждения с происхождением; не распространять сильную роль на соседей.

| Основание | Перенос |
|---|---|
| Явный governance/binding target с восстановимой областью ограничения | кандидат `constrains` с тем же selector; действие зависит от §4 |
| Drift-watch target | observation/change watch; сам по себе не нормативное ограничение |
| implementation_footprint.files | navigation с `legacy_role: implementation_footprint` |
| affected_files/affected_symbols Note | navigation, никогда constrains |
| Старый affected_* без явной роли | legacy navigation; отсутствие footprint не доказывает governance |
| Старый hash/path/symbol не разрешается теперь | сохранить reference и диагностировать scope/currentness, не терять запись |

Сохранять file_hash, symbol_hash, kind, line/end-line, receiver/signature/anchor metadata,
когда они есть. Хэш старого тела символа не выдавать за signature hash нового адреса.
Разрешение имени в сегодняшнем коде не доказывает историческую идентичность символа.
`existing_unresolved` требует проверки разрешения в исходном контексте; иначе это
`resolution_unknown`/`new_unresolved` с причиной, а не ложное точное совпадение.

Код основания: `internal/artifact/decision_scope.go:10–28` различает роли;
`internal/artifact/decision.go:1519–1522` не создаёт baseline из footprint-only;
`internal/artifact/types.go:579–625` содержит footprint/targets и исходные поля binding.

## 7. Секции, claims и термины

30 DB-секций содержат 224 claims: 109 L, 42 A, 65 D, 8 E. Scope есть у всех 224;
support_refs у 148, governing_pattern_refs у 147, evidence_refs у 10; секции имеют 36 target_refs.
Числа полей здесь означают число владельцев с полем, а не число элементов массивов.

- Сохранить section/claim IDs и полную исходную claim edition; новые slug/aliases не
  должны перенаправить старое свидетельство на новый текст. Указывать источник каждого ID.
- Не считать target_refs или carrier path предметом секции автоматически. Если about
  не восстановлен, сохранить исходник с `needs_rewrite`; не подставлять system:haft.
- Старый L не обязательно definition из-за section.statement_type; A не обязательно
  исполняемый guard; D не обязательно лишь policy. Сохранить класс/statement/scope в
  legacy и формировать новый kind только по восстановимому смыслу. Иначе needs_rewrite.
- 8 E-claims сохранить с их ID и содержанием в historical claim material. В новую spec
  как normative claim не включать; не создавать достоверное evidence из одного E-ярлыка.
- support_refs переводить в refs только после разрешения адреса и проверки нового
  значения/направления. Иначе legacy_refs, видимые в recall/context, и причина в отчёте.
- governing_pattern_refs сохранять как исторические ссылки. Не назначать SHA текущего
  FPF. Если исходный SHA неизвестен, `source_revision: {kind: unknown, reason: ...}`
  по CONTRACT, не `sha: legacy`.
- evidence_refs не заменять новыми supports. valid_until сохранить как source window;
  owner/baseline approved_by — исторические поля, не новая подпись или одобрение.
- Baselines не превращать в доказательство проверки. Отсутствующие checks честно
  помечать unchecked; ни type/test reference, ни covers не изобретать.
- Target-секции не ужимать механически в четыре поля с удалением claims. Сохранить
  исходные адресуемые утверждения; новый concept summary — отдельная последующая работа.
- Terms переносить с definition/aliases/not и происхождением. При неоднозначном имени
  сохранить конфликт; автоматически выбранный domain qualifier меняет reference scheme.

## 8. Evidence: наблюдение, область и историческая редакция

Обе таблицы обязательны. Первичный ключ миграционной строки — `(source_table, id)`;
если в иной установке ID пересекутся, использовать явный alias/conflict, не overwrite.

| Verdict в двух таблицах | Количество | Новая проекция |
|---|---:|---|
| supports | 222 | supports в исходной области, только с установленной target edition |
| weakens | 35 | weakens, не inconclusive |
| refutes | 4 | weakens + legacy_verdict: refutes; отметить огрубление |
| supports_* | 5 | Сохранить exact verdict; не все автоматически supports |
| superseded | 26 | legacy retirement cue, не свежее evidence |
| accepted | 23+7 | inconclusive + legacy_verdict |
| partial | 4+3 | inconclusive + legacy_verdict |
| pass / bounds_scope / limits_scope / comparison_baseline / пусто | 2/1/1/1/5 | inconclusive с точным старым значением/отсутствием |

`supports_*`: supports_deadline_comparison, supports_first_slice,
supports_precision_first_target, supports_reframe, supports_with_residual_routing_debt —
по одному. Если код не восстанавливает их ограниченный target/use, сохранить наблюдение
и legacy verdict без широкой положительной поддержки. У 2 evidence цель Portfolio→options.

178 evidence_items и 2 старых evidence имеют claim_scope; 84 evidence_items имеют
claim_refs. Scope переносится буквально с владельцем. Claim IDs разрешать внутри
исторической записи, не глобально; при неизвестном соответствии сохранять legacy targets,
не применять поддержку к целому decision. Это ограничение должно быть видно потребителю,
не только человеку в отчёте. Поддержка актуального claim отсутствует до восстановления basis.

**Нельзя закрепить старое evidence на хэше свежеперенесённого target и объявить его
поддержкой этой редакции.** Hash миграции идентифицирует результат конверсии, не доказывает
historical claim edition. Хранить edition unknown и историческую навигацию; свежая проверка
создаёт отдельное применение evidence. Коммит кода тоже не подменяет редакцию утверждения.

Сохранить content, type (55 значений в evidence_items), carrier_ref, provenance, timestamps,
claim_scope/refs, valid_until, formality/congruence и иные исходные populated поля в legacy,
даже если v10 не считает старые scores. Type без описания метода не превращается в Method.
137 evidence_items и 2 evidence имеют carrier_ref; у старых 10 carrier_commit/hash не заполнены.
Отсутствующий кодовый SHA не заполнять HEAD. basis может быть report/data/period/other;
неизвестный basis показывается как неизвестный, независимо от формы about.

## 9. Отчёт и переключение одной командой

Отчёт содержит source snapshot/DB identity, scope, counts, параметры, output path; для
обнаруженной основной записи и значимой потери — source table+key, старое значение или
recoverable locator, destination, новое значение, reason и disposition. Исключённые
метаданные можно показать группой по MIGRATION-POLICY. Печатный итог сопровождается
машиночитаемым отчётом; это выход той же команды, не отдельный продукт проверки.

| Disposition | Смысл |
|---|---|
| preserved | смысл действительно перенесён, не только скопировано число строк |
| weakened | новый потребитель получает более слабое утверждение; оригинал сохранён |
| needs_rewrite | оригинал доступен, новое содержательное утверждение не создано |
| existing_unresolved | разрыв подтверждён в исходных данных |
| not_carried_by_policy | исключённый вид/его связь, решение Ивана, counts и ID без архива 935 записей |
| field_loss / record_loss | не попало в результат v10; affected use, причина и locator оригинала перечислены |
| source_unreadable / unsupported_source | область не прочитана/не поддержана; полнота и её counts не выдумываются |

Не делать ослабление/неизвестность эквивалентом успешной активации. Известные пропуски
в результате v10 не блокируют переход после обозримого отчёта и выбора пользователя;
не добавлять скрытый zero-loss gate по kind. Повреждение результата миграции, отсутствие
защищённого исходника, неразрешённая коллизия, активные старые writers и перезапись новых
v10-данных блокируют switch. Неизвестную область нельзя выдать за прочитанную; её влияние
видно в выборе поддерживаемой части или чистого старта. Потерянное для v10 содержание
можно восстановить позднее с агентом как proposed. Очередь не блокирует всю установку.
Открытый выбор о применении legacy active блокирует только соответствующее live-действие.

`--dry-run` читает источники и строит staged outcome/report без остановки процессов,
переключения `.haft`, изменения БД, установки или коммита. Реальный запуск сначала
останавливает/дренирует идентифицированных writers именно этого проекта; stale PID не
разрешает kill по имени. Затем повторно получает согласованный read-only snapshot DB,
учитывающий WAL, и digest носителей; предыдущий dry-run не является свежим snapshot.

Выход строится вне live `.haft`; повторный запуск не затирает чужой каталог. До switch
сверить, что writers не вернулись и source/carrier basis не изменился. Сохранить исходную
базу и носители для возврата. Switch выполнять при остановленных читателях/writers с
восстановлением после частичного шага; один Git-коммит не делает несколько filesystem
операций транзакцией. Коммит включает только согласованный migration output, не посторонний WIP.
После первых записей v10 возврат бинарника 9.x сам по себе не переносит новую память обратно.

## 10. Запросы для повторной инвентаризации

```sql
SELECT kind, status, count(*) FROM artifacts GROUP BY kind, status;
SELECT link_type, count(*) FROM artifact_links GROUP BY link_type;
SELECT l.link_type, s.kind, coalesce(t.kind,'UNRESOLVED'), count(*)
FROM artifact_links l LEFT JOIN artifacts s ON s.id=l.source_id
LEFT JOIN artifacts t ON t.id=l.target_id GROUP BY 1,2,3;
SELECT a.kind, count(*) FROM affected_files f
JOIN artifacts a ON a.id=f.artifact_id GROUP BY a.kind;
SELECT verdict, count(*) FROM evidence_items GROUP BY verdict;
SELECT verdict, count(*) FROM evidence GROUP BY verdict;
SELECT count(*) FROM artifacts WHERE kind='DecisionRecord'
AND json_valid(structured_data)
AND coalesce(json_array_length(json_extract(structured_data,
  '$.implementation_footprint.files')),0)>0;
```

Для fields/claim statistics читать JSON напрямую, сохраняя distinction missing/empty.
Для content проверять raw bytes до UTF-8 decode. Эти запросы — воспроизводимая инвентаризация,
не отдельная команда-сравнитель и не свидетельство уже прошедшего миграционного теста.
