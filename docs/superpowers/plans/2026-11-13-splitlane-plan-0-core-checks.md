# SplitLane, план 0: проверки ядра на ПК — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Проверить на настоящем ядре и настоящей системе допущения спецификации (а), (б), (в), (г), (д), (з), (и), (к) из раздела 12, записать результаты в приложение A и принять три решения, от которых зависит план 1.

**Architecture:** Задачи этого плана — опыты, а не код продукта. Одноразовые скрипты в `spike\plan0\` запускают неизменённое ядро sing-box 1.14.4. Ядро без адаптера запускается напрямую и без прав администратора; ядро с адаптером — только через надзорный скрипт `spike\scripts\watchdog.py`, проверенный утром 2026-11-13: он сам возвращает системный VPN при ошибке, по команде и при потере связи с исполнителем. Каждая проверка печатает строки `PASS` / `FAIL` / `INFO` и пишет запись в `spike\plan0\out\results.json`; последняя задача собирает из записей текст для приложения A.

**Tech Stack:** sing-box 1.14.4 (официальная сборка windows-amd64), Tailscale 1.104.2, Windows 10 Pro 19045, Windows PowerShell 5.1, Python 3.14, Node 24 (только как тестовый процесс).

**Spec:** `docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md` — разделы 5.3–5.7, 6, 7, 8.2, 12 (пункт «Проверки на ПК до основного кода») и приложение A.

**Что не входит.** Проверки (е) (запуск через планировщик задач) и (ж) (завершение сеанса Windows) требуют кода на Go. Они становятся первыми задачами плана 1 (служебный процесс). Провал (е) по спецификации тоже останавливает работу, но решается это уже там.

**Что получается на выходе:**

1. Записи в приложении A спецификации, без адресов и имён.
2. Решение по 6.3: переключатели через режим ядра (`clash_mode`) или запасной способ (отдельные перечитываемые файлы правил).
3. Решение по разделу 7: служба API ядра как источник ссылки для входа, состояния узла и списка exit node или запасной способ (журнал ядра и системный клиент).
4. Решение по 8.2: исключения для вложенных правил через логическое правило с `invert` в перечитываемом файле или запасной способ (правила 9 и 10 меняются местами).

## Global Constraints

- Ядро: только sing-box 1.14.4, файл `C:\code\router\spike\dl\sing-box-1.14.4\sing-box-1.14.4-windows-amd64\sing-box.exe`, без изменений.
- Всё одноразовое лежит в `C:\code\router\spike\` (строка `spike/` в `.gitignore`). В git попадают только этот план и правка приложения A.
- Личные данные. В план, спецификацию и коммит не попадают имена устройств и сети Tailscale, настоящие адреса, секреты API, ссылка для входа. Конфиги в плане содержат метки `__EXIT_NODE__`, `__TS_HOSTNAME__`, `__TS_STATE_DIR__`, `__CLASH_SECRET__`, `__API_SECRET__`, `__DEFAULT_MODE__`; настоящие значения подставляет `render()` из `p0lib.py` во время работы, из локальных файлов `spike\run\phase-b-v2.json` (блок `endpoints`), `spike\run\clash-secret.txt`, `spike\scripts\watchdog.py` (постоянные `EXIT_NODE`, `EXPECT_IP`). Скрипты печатают названия дорожек `VPN` / `DIRECT`, а не адреса.
- Ядро с адаптером запускает только watchdog. Команду `tailscale set --exit-node=...` никто не выполняет руками, пока ядро может быть живо: включённый системный exit node при живом адаптере блокирует все программы, а процесс без прав администратора не может завершить ядро.
- Запуск watchdog и каждая команда `engage` / `restart` / `stop` — один вызов инструмента PowerShell с тайм-аутом 600000 мс. Ожидание итога по `status.json` идёт внутри того же вызова; принимается только свой идентификатор запуска и своей команды; при тайм-ауте ожидания пишется `stop.flag`. Это делают функции `Start-P0Watchdog` и `Send-P0Command`.
- Пока ядро поднято, каждая команда исполнителя начинается с `Set-P0Heartbeat`: в `spike\run\deadline.txt` через временный файл записывается «сейчас + 400 секунд» в секундах Unix. Время берётся из `[DateTimeOffset]::UtcNow.ToUnixTimeSeconds()`; `Get-Date -UFormat %s` на этом ПК даёт местное время с десятичной запятой и не используется. Между командами не должно проходить больше 400 секунд.
- Запрос UAC принимает человек. Такие шаги помечены «нужен пользователь: UAC». Перед каждым запросом исполнитель предупреждает пользователя: на ответ есть 120 секунд. Отменённый запрос ничего не меняет в системе; повторять его без согласия пользователя нельзя. При безошибочном прохождении запросов два: задача 3 и задача 5.
- Шаги с пометкой «нужен пользователь» выполняет только исполнитель, который сам ведёт разговор с пользователем; подчинённому агенту они не передаются (раздел «Как читать этот план», абзац «Кто выполняет»).
- В правила прямой дорожки попадают только переименованные копии: `curl-*.exe` и `node-*.exe`. `claude.exe`, `node.exe`, `python.exe`, `bun.exe`, `svchost.exe` и браузеры в прямое правило не попадают никогда.
- Основная дорожка каждого конфига — VPN (`final` = `vpn`). Исключение — короткие под-шаги с режимами `direct`, `direct-localdns`, `alldirect`; пользователь заранее даёт на них согласие (задача 3, шаг 2). Перед ними `confirm_pin()` подтверждает, что программы исполнителя (`claude.exe`, `bun.exe`, `node.exe`, `python.exe`, `pythonw.exe`) закреплены за VPN правилом `process_name` (правило 8), которое стоит перед всеми правилами режимов. Сразу за ним правило 9 закрепляет за VPN адреса сервиса, через который работает исполнитель (`ip_cidr` `160.79.104.0/23`, `domain_suffix` `anthropic.com`, `claude.ai`, `claude.com`): оно удерживает на VPN соединения, владельца которых ядро не определило. Каждый такой под-шаг заканчивается явным возвратом в режим `vpn`. Правила 8 и 9 есть только в проверках: в продукте режим «всё напрямую» таких исключений не имеет (кроме служебного процесса SplitLane).
- Запись результата проверки, которой нужен сеанс watchdog, делает только `record_live()`: если сеанс к этому моменту закончился, скрипт печатает `ABORT SESSION-LOST` и ничего не записывает. Откат сеанса — событие проверочной оснастки, а не свойство ядра.
- Файлы правил и конфиги пишутся в UTF-8 без метки порядка байтов, атомарно: временный файл в той же папке, затем переименование. В PowerShell 5.1 `Set-Content -Encoding utf8` и `>` пишут метку или UTF-16; используется только `[IO.File]::WriteAllText` с `New-Object Text.UTF8Encoding $false`.
- Файлы PowerShell содержат только ASCII: PowerShell 5.1 читает скрипт без метки в кодировке ANSI. Кириллица встречается только в файлах Python.
- Постоянные адреса проверок: вход `probe-vpn` на `127.0.0.1:17891` (его ждёт watchdog), вход `probe-direct` на `127.0.0.1:17892`, Clash API на `127.0.0.1:19090`, служба API на `127.0.0.1:19091`. Журнал ядра для конфигов watchdog — `C:/code/router/spike/run/phase-b.log`: watchdog ищет строку запуска именно в нём.
- Пределы watchdog: 45 минут с момента включения, 15 минут удержания (`hold`), три неудачные проверки связи подряд (девять в задаче 5).
- Текст плана и приложения A — по-русски; команды, код и вывод скриптов — по-английски.

**Ручное восстановление.** Этот текст пользователь получает до первого шага с UAC (задача 1, шаг 1). Исполнитель показывает его снова при итоге `restore_unverified`, при строке `NO-TERMINAL-STATE` и при строке `WRONG` в проверке системы после сеанса.

```text
Если интернет в программах пропал и не вернулся за 10 минут:
1. Нажмите значок Tailscale в трее, откройте список Exit Nodes и выберите прежний exit node.
2. Откройте Диспетчер задач (Ctrl+Shift+Esc), вкладка «Подробности», найдите sing-box.exe и нажмите «Снять задачу».
Порядок важен: сначала exit node, потом sing-box.exe.

Если компьютер во время проверок перезагрузился, выключился или завис и был перезапущен:
после входа в Windows VPN может оказаться выключенным, и тогда программы идут в интернет напрямую без всякого предупреждения.
Сразу нажмите значок Tailscale в трее и, если exit node не выбран, выберите прежний. Искать sing-box.exe в этом случае не нужно.
```

**Состояния watchdog и что делать.** Функции `Start-P0Watchdog`, `Send-P0Command` и `Close-P0StaleSession` заканчиваются строкой `P0-RESULT:`.

| Строка `P0-RESULT:` | Что это значит | Действие |
|---|---|---|
| `state=holding` | ядро с адаптером без перехвата поднято, системный exit node не тронут | продолжать |
| `state=ready` | деление включено, системный exit node выключен, watchdog следит | продолжать |
| `state=ready ... cmd_error=...` или `state=holding ... cmd_error=...` | команда отклонена, работает прежний конфиг | прочитать причину, исправить, повторить команду |
| `UAC-CANCELLED` | запрос UAC отменён, ничего не запущено | спросить пользователя; без согласия не повторять |
| `REFUSED - ...` (от `Start-P0Watchdog` и `Send-P0Command`) | запуск или команда не начаты, система не тронута | устранить причину из текста |
| `state=failed_no_change` | watchdog отказался до любых изменений; причина в `reason` | устранить причину; новый запуск — новый запрос UAC, спросить пользователя |
| `state=hold_failed` или `state=hold_done` | удержание закончено, системный exit node не трогался | причина в `reason` |
| `state=rolled_back ... system_vpn_ok=True` | откат выполнен, системный VPN работает | причина в `reason`; новый запуск — новый запрос UAC |
| `state=restore_unverified` | exit node включён обратно, но проверка связи не прошла | показать пользователю ручное восстановление |
| `NO-LIVE-SESSION` | сеанса нет: он уже закончился | выполнить `Get-P0Session`, прочитать причину |
| `NO-LIVE-SESSION - stale status ...` | процесса watchdog нет (ПК перезагружался, или процесс сняли), а в `status.json` осталось прежнее состояние; при команде `stop` на всякий случай записан `stop.flag` | выполнить `prepare.py`: строки `STATE no sing-box.exe running` и `STATE system exit node on` должны быть `OK`, иначе показать пользователю ручное восстановление и повторить; затем `Close-P0StaleSession` и снова `prepare.py` |
| `state=restore_unverified ... reason=stale status closed ...` | итог `Close-P0StaleSession`: устаревшее состояние закрыто | `prepare.py` должен закончиться `PREPARE-OK` |
| `NOTHING-TO-CLOSE` | итог `Close-P0StaleSession`: сеанс уже в конечном состоянии | продолжать |
| `REFUSED - a sing-box.exe is running ...` (от `Close-P0StaleSession`) | ядро работает без надзора | показать пользователю ручное восстановление, затем повторить команду |
| `REFUSED - a python.exe older than the last status is running ...` | watchdog, возможно, ещё работает: есть `python.exe` старше последнего состояния, у которого в командной строке стоит `watchdog.py` либо командная строка не читается (так выглядит процесс с правами администратора). Чужие `python.exe` с читаемой командной строкой без `watchdog.py` не считаются | ничего не закрывать; через 10 минут выполнить `Get-P0Session` (живой watchdog за это время сам откатится по `deadline.txt`); если состояние не изменилось — спросить пользователя, не запущена ли у него программа на Python с правами администратора, и повторить команду после её закрытия |
| `NO-TERMINAL-STATE` в строке `P0-STATUS:` | watchdog не ответил на `stop.flag` | показать пользователю ручное восстановление |
| `no status` и строка `P0-STATUS: NO-STATUS` | watchdog этого запуска ни разу не отчитался (не стартовал или сразу упал); система не тронута | убедиться, что нет процессов `sing-box.exe` и повышенного `python.exe` с `watchdog.py`; причину искать в `spike\run\watchdog.log` |

Устаревшее состояние закрывает только `Close-P0StaleSession`; обходного ключа у `Start-P0Watchdog` нет. Команда:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Close-P0StaleSession
```

**Строки `ABORT` скриптов Python.** `ABORT SESSION-NOT-READY` — сеанс не в нужном состоянии уже к началу скрипта (или ядра нет, а `status.json` устарел; или ядро запущено не с тем конфигом, который нужен скрипту). `ABORT SESSION-LOST` — сеанс закончился, пока скрипт работал; записи этого шага не сделаны. `ABORT MEASUREMENT-FAILED` — запросы остались без ответа либо не прошёл контроль (шаг 7 задачи 3), оценить нечего; запись этого измерения не сделана. В первых двух случаях причину показывает команда:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Get-P0Session
```

## Review Focus

Условия, с которыми опыты встретятся на живой машине и которые сами проверки не покрывают, от более вероятного к менее вероятному.

1. Пользователь отменил запрос UAC или ответил позже 120 секунд. Ожидание: ничего не запущено, система не изменена, повтор только с согласия пользователя. Закреплено в задаче 1, шаг 8: строки `ok   cancelled UAC starts nothing` и `ok   second launch refused`; поздний ответ отклоняет сам watchdog (аргумент `--not-after`).
2. Исполнитель потерял связь при поднятом ядре и не обновил `deadline.txt`. Ожидание: watchdog сам возвращает системный exit node не позже чем через 400 секунд. Закреплено в задаче 1, шаг 8: строка `ok   watchdog: hold, deadline expired`; для включённого ядра то же условие проверяет цикл надзора watchdog (причина отката `heartbeat deadline expired`), который правка не меняет.
3. Ядро не определило владельца одного соединения, и ответ пришёл не с той дорожки; либо запрос или пакет UDP остался без ответа. Ожидание: один ответ не с той дорожки не считается провалом проверки, два из трёх — считаются; запрос без ответа повторяется и ответом не считается; один потерянный пакет UDP не становится записью `FAIL`, а настройка `dns.reverse_mapping` не выключается по одному прогону. Закреплено в задаче 1, шаг 8: строка `ok   p0lib: two-of-three rule for lane answers`; в задаче 3, шаг 1: строка `ok   check_revmap: a lost datagram is sent again; FAIL needs a wrong lane or three losses in a row`; в задаче 3, шаг 8: таблица исходов трёх прогонов.
4. Кэш DNS Windows отдал ответ, и запрос не дошёл до ядра. Ожидание: проверка сервера DNS не зависит от кэша. Закреплено в задаче 3, шаги 5 и 6: запрос отправляется прямо на адрес DNS адаптера, а сообщение `dns: query not seen in the core log` считается провалом.
5. В коммит попали адрес, имя устройства или секрет. Ожидание: коммит не создаётся. Закреплено в задаче 6, шаг 5: `report.py scan` должен напечатать `PRIVACY-OK`.

---

## Как читать этот план

**Порядок задач.** Задача 1 готовит файлы и не трогает систему. Задача 2 запускает ядро без адаптера и без UAC. Задача 3 — один сеанс watchdog с одним запросом UAC. Задачи 4 и 5 требуют рук пользователя; их провал или пропуск не мешает плану 1. Задача 6 записывает результаты и выполняется всегда, даже если работа остановлена раньше.

**Кто выполняет.** Шаг 1 задачи 1 и задачи 3, 4 и 5 целиком выполняет исполнитель, который сам ведёт разговор с пользователем (основной сеанс, superpowers:executing-plans). Подчинённому агенту их передавать нельзя: у него нет канала к пользователю. Он не может показать ручное восстановление и дождаться ответа, получить согласие на шаги с прямой основной дорожкой, предупредить о запросе UAC (120 секунд на ответ), передать тексты задач 4 и 5; его фоновые процессы завершаются вместе с ним. Подчинённым агентам можно отдать только шаги 2–11 задачи 1, задачу 2 и задачу 6. Задаче 6 основной сеанс передаёт наблюдения задач 2–5, которых нет в записях (шаг 4 задачи 6), а сообщение из её шага 7 сам передаёт владельцу продукта.

**Сеанс watchdog не переходит из задачи в задачу.** Он начинается и заканчивается внутри одной задачи. Задачу 3 выполняет один исполнитель без перерывов.

**Инструмент.** Команды в блоках `powershell` рассчитаны на инструмент PowerShell (Windows PowerShell 5.1, запускается с `-ExecutionPolicy Bypass`). Человеку для тех же команд нужно окно `powershell -ExecutionPolicy Bypass` без прав администратора. Файл плана больше 256 КБ и не помещается в один вызов инструмента чтения: читайте его частями (параметры `offset` и `limit`, по 500 строк). Блок кода переносится в файл целиком, от строки после открывающих трёх обратных апострофов до строки перед закрывающими; после записи файла сверяйте его последнюю строку с последней строкой блока.

**Папка `spike\plan0\`.** После подготовки плана в ней могут лежать файлы с теми же именами и папка `author-check\`. Источник истины — текст плана: каждый файл записывается заново из плана. Папка `author-check\` в плане не используется, её можно удалить.

**Строки результата.** `PASS` — подтверждено. `FAIL` — не подтверждено. `INFO` — наблюдение без оценки. `SKIP` — часть проверки не выполнялась, записи нет. `ABORT` — скрипт отказался работать, после слова указана причина; виды `ABORT`, связанные с сеансом, перечислены в Global Constraints.

### Карта файлов

| Файл | Назначение | Задача |
|---|---|---|
| `spike\plan0\lib.ps1` | PowerShell: `Set-P0Heartbeat`, `Start-P0Watchdog`, `Send-P0Command`, `Get-P0Session`, `Close-P0StaleSession` | 1 |
| `spike\plan0\p0lib.py` | Python: подстановка меток, файлы правил, Clash API, служба API, дорожки, журнал ядра, запись результатов | 1 |
| `spike\plan0\patch_watchdog.py` | точная правка `spike\scripts\watchdog.py` | 1 |
| `spike\plan0\tpl\setup.json`, `hold.json`, `main.json` | шаблоны конфигов с метками | 1 |
| `spike\plan0\selftest\*` | самопроверка без ядра и без UAC | 1 |
| `spike\plan0\prepare.py` | состояние ПК, копии программ, готовые конфиги | 1 |
| `spike\plan0\check_setup.py` | (д), (б) в режиме настройки | 2 |
| `spike\plan0\check_hold.py` | (г), (б) до переключения | 3 |
| `spike\plan0\check_modes.py` | (а) | 3 |
| `spike\plan0\check_rules.py` | (в) | 3 |
| `spike\plan0\check_revmap.py` | (к) | 3 |
| `spike\plan0\check_api.py` | (б) в обычном режиме | 3 |
| `spike\plan0\check_relogin.py` | (з) | 4 |
| `spike\plan0\observe.py` | (и) | 5 |
| `spike\plan0\report.py` | таблица, решения, текст приложения A, поиск личных данных | 6 |
| `spike\run\p0-*.json`, `spike\plan0\out\*`, `spike\plan0\rules\*`, `spike\plan0\bin\*`, `spike\plan0\api-secret.txt` | создаются скриптами | — |

### Что уже проверено при подготовке плана

Без запуска ядра, без watchdog, без UAC и без изменения настроек:

- Каждый конфиг из плана прошёл `sing-box check -D C:\code\router\spike\run` с безвредными значениями вместо меток: `p0-setup.json`, `p0-setup-fresh.json`, `p0-hold.json`, `p0-main-vpn.json`, `p0-main-vpn-localdns.json`, `p0-main-direct.json`, `p0-main-direct-localdns.json`, `p0-main-alldirect.json`, `p0-main-vpn-norevmap.json`.
- Все варианты файлов правил из `check_rules.py` прошли ту же проверку: пробелы, скобки, кириллица, логическое правило с `invert`. Намеренно испорченный файл ядро отвергает с текстом `parse rule_set.rules.[7]: process_path_regex: parse expression 0: error parsing regexp: missing closing )`.
- Подкоманды `sing-box api` есть в официальной сборке (проверено командами `--help`).
- `patch_watchdog.py` применён к копии `watchdog.py`; копия прошла самопроверку из шага 8 задачи 1.
- `lib.ps1` прошёл самопроверку из шага 8 задачи 1.
- Скрипты `check_*.py` и `observe.py` выполнены целиком на имитации ядра: при исправной имитации все записи `PASS`; при внесённых отказах (не работает `invert`, не работает сравнение без учёта регистра, правило DNS не срабатывает, закрепление программ исполнителя сломано, режим не переключается) соответствующие записи `FAIL`, а режим в конце возвращён в `vpn` везде, где ядро вообще переключается. Ещё три случая имитации: сеанс откатился посреди скрипта — `ABORT SESSION-LOST`, записей этого шага нет; запросы копий остались без ответа — `ABORT MEASUREMENT-FAILED`, записей нет, режим возвращён в `vpn`; сервер DNS выбран верно, но не отвечает — `FAIL` с текстом `dns-no-answer`, который в решение 1 не входит.
- Правила записи `K3` (шаг 8 задачи 3) проверены на той же имитации. Один потерянный пакет — `PASS` с текстом `answered only when sent again`. Четыре строки таблицы исходов: большие пакеты теряются только при включённом обратном сопоставлении — `FAIL`, `PASS`, `FAIL` в трёх прогонах; теряются всегда — три `FAIL`; терялись только в первом прогоне — `FAIL`, `PASS`, `PASS`; терялись в первом и в контрольном прогонах — `FAIL`, `FAIL`, `PASS`. По дорожке VPN не отвечает ни один размер — `ABORT MEASUREMENT-FAILED`, записи `K3` нет, запись `K4` есть. Перезапуск с контрольным конфигом не состоялся — `ABORT SESSION-NOT-READY`, записей нет. Самопроверка `check_revmap.py selftest` (шаг 1 задачи 3) проходит и падает с `AssertionError` при каждой из пяти внесённых в правила ошибок: нет повторной отправки; повторно отправляется всё; отсутствие ответов считается оценкой; потерянный размер считается пройденным; ответ с другой дорожки считается пройденным.

- На той же имитации проверены случаи, добавленные после третьего круга проверки плана. Режим `direct` читается обратно, но основная дорожка прямой не становится — записи (а) `FAIL`, вместо записи `V5` строка `SKIP ... mode direct did not give a direct main lane`. Файл дорожки VPN не срабатывает при прямой основной дорожке — `FAIL V5`. Не работает `invert` — `FAIL V4`, а `V5` — `PASS` с текстом `nested copy not judged: V4 failed`. Контроль `V0` не проходит — `ABORT MEASUREMENT-FAILED`, записей `V1`–`V9` нет. Watchdog перед переключением измерил другой адрес или не получил ответа — `FAIL G3`. Проверочное имя перестало быть псевдонимом — `SKIP K5`, записи нет.
- `observe.py` выполнен на восьми заданных ходах событий с подставными часами и подставными проверками дорожек: кабель с возвратом дорожки VPN через 10 секунд после сети — `PASS` и `PASS` для записи `-direct`; то же с переходом соединения на ретранслятор — `PASS` и `FAIL`; соединение через ретранслятор ещё до события — `PASS` и `INFO`; одна неудачная проверка в начале и настоящее событие с отставанием 130 секунд — `FAIL`; сон 84 секунды — `PASS`; откат после девяти неудач — `FAIL`; откат с причиной `traffic left through` — `FAIL` с признаком ухода трафика, адрес в записи закрыт; ничего не произошло — `INFO` без записи `-direct`.
- `report.py` выполнен на составленных записях: провал только `A3` оставляет решение 1 основным; `FAIL Z5`, `FAIL Z3-link-after-restart`, `K3` без снятого контролем провала и запись `I-...` с признаком ухода трафика попадают в строку `OWNER-QUESTION`; пропущенная `V5` при провале (а) даёт решение 1 «запасной способ» без строки остановки; без опыта с Wi-Fi пункт «смена сети Wi-Fi ↔ кабель» остаётся в абзаце «Не проверялось».
- Три сценария самопроверки watchdog, добавленные вместе с исправлениями третьего круга (`engage, core lost`, `engage, slow probe`, `hold, full adapter`), не проходят на правке без этих исправлений. Проверка устаревшего состояния (`Test-P0WatchdogGone`) на этом ПК не считает чужие процессы `python.exe` и считает процесс, в командной строке которого есть `watchdog.py`.

Сами допущения спецификации этим не проверены: их проверяют задачи 2–5. Скрипты проверок ни разу не работали с настоящим ядром; первая же задача с ядром может показать ошибку в скрипте, а не в ядре. Признак такой ошибки — `Traceback` или `ABORT` (кроме трёх видов из Global Constraints) вместо строк `PASS` / `FAIL`; её исправляют в скрипте и повторяют шаг. При поднятом ядре порядок такой: сначала команда `reset` из правил задачи 3, потом исправление скрипта; если исправление займёт больше трёх минут — сначала шаг 12 задачи 3, а после исправления новый сеанс.

### Факты о ядре 1.14.4, на которые опирается план

Источник — исходный код и документация по метке `v1.14.4` (`https://github.com/SagerNet/sing-box/tree/v1.14.4`); в скобках путь файла.

- `clash_mode` в правиле маршрута и в правиле DNS — одна строка, сравнение без учёта регистра (`option/rule.go`, `option/rule_dns.go`, `route/rule/rule_item_clash_mode.go`).
- Список режимов — режимы из правил маршрутов и DNS плюс режим по умолчанию; `experimental.clash_api.default_mode`, при пустом значении `Rule` (`experimental/clashmode/mode_list.go`, `experimental/clashmode/manager.go`, `box.go`).
- `PATCH /configs` с телом `{"mode": "..."}` всегда отвечает 204; неизвестный режим молча игнорируется; `GET /configs` отдаёт `mode` и `mode-list` (`experimental/clashapi/configs.go`).
- При смене режима ядро очищает свой кэш DNS и обратное сопоставление и пишет строку `clash-mode: updated mode: <режим>`; открытые соединения не трогает; кэш DNS Windows не сбрасывает (`experimental/clashmode/manager.go`, `dns/router.go`, `protocol/tun/inbound.go`).
- Служба API: `services: [{ "type": "api", "listen": ..., "listen_port": ..., "secret": ... }]`; gRPC, заголовок `authorization: Bearer <секрет>`; панель управления выключена, пока не задано поле `dashboard` (`docs/configuration/service/api.md`, `option/api.go`, `service/api/server.go`, `daemon/server.go`).
- Клиент службы API встроен в ядро: `sing-box api`, адрес и секрет — флаги `--url`, `--secret` или переменные `BOX_API_URL`, `BOX_API_SECRET`; подкоманды `tailscale status`, `tailscale exit-node list`, `tailscale ping <узел>`, `tailscale logout`, `mode`, `connection list`. Когда вывод идёт не в терминал, таблицы печатаются строками через табуляцию без заголовка (`cmd/sing-box/cmd_api.go`, `cmd_api_output.go`, `cmd_api_tailscale_status.go`, `cmd_api_tailscale_exit_node_list.go`, `cmd_api_tailscale_ping.go`).
- Данные службы API об узле: состояние, ссылка для входа, свой узел, узлы сети с признаками «в сети» и «может быть exit node», выбранный exit node; ответ `ping` различает прямое соединение и ретранслятор (`daemon/started_service.proto`: `TailscaleEndpointStatus`, `TailscalePeer`, `TailscalePingResponse`).
- Ссылка для входа пишется в журнал на уровне `info`: `endpoint/tailscale[<тег>]: Waiting for authentication: <ссылка>` (`protocol/tailscale/endpoint.go`).
- Правила без действия в файле правил: поля `process_path_regex`, `invert`; логическое правило `type: "logical"`, `mode: "and"` или `"or"`, `rules`, `invert`; версии формата 1–5, версии 3 достаточно (`option/rule_set.go`, `docs/configuration/rule-set/headless-rule.md`, `docs/configuration/rule-set/source-format.md`).
- `process_path_regex` — регулярные выражения Go; путь процесса в Windows берётся из `QueryFullProcessImageName`, для процесса 4 это `:System`; `process_name` — точное имя файла с учётом регистра (`route/rule/rule_item_process_path_regex.go`, `common/process/searcher_windows.go`, `route/rule/rule_item_process_name.go`).
- Локальный файл правил перечитывается при изменении; при ошибке ядро пишет `router: reload rule-set <тег>: <причина>` и оставляет прежние правила; об успехе не сообщает (`route/rule/rule_set_local.go`).
- Поле правила `inbound` — список тегов входов (`route/rule/rule_item_inbound.go`).
- В одном правиле маршрута `domain_suffix` и `ip_cidr` входят в одну группу «адрес назначения»: правило срабатывает, если подошло любое из них (`route/rule/rule_abstract.go`, `docs/configuration/route/rule.md`). Имя у соединения без разбора протоколов появляется только из обратного сопоставления, поэтому при выключенном `dns.reverse_mapping` в таком правиле работает только `ip_cidr`.
- `dns.reverse_mapping` запоминает адреса из ответов DNS; при выборе маршрута имя записывается в соединение, строка журнала уровня `debug`: `router: found reserve mapped domain: <имя>` (`dns/router.go`, `route/route.go`).
- Clash API, `GET /connections`: `id` — идентификатор соединения, `metadata.host` — имя соединения, `metadata.processPath` — путь процесса, `chains` — теги выходов, `rule` — текст правила или `final`; `DELETE /connections/<id>` закрывает одно соединение и всегда отвечает 204 (`experimental/clashapi/connections.go`).
- `sing-box api tailscale exit-node list` печатает узлы сети, кроме своего, у которых есть признак «может быть exit node»; столбцы: имя, первый адрес узла, «в сети» (`yes` или `no`), `selected` или `-` (`cmd/sing-box/cmd_api_tailscale_exit_node_list.go`, `cmd_api_tailscale_peer.go`, `cmd_api_output.go`).
- Строки журнала, которые читают скрипты: `router: found process path: <путь>`, `router: match[<номер>] <правило> => route(<выход>)`, `dns: exchange <имя>. IN A`, `dns: match[<номер>] clash_mode=<режим> => route(<сервер>)` (`route/route.go`, `dns/router.go`, `route/rule/rule_action.go`).
- Сервер DNS типа `local` в Windows берёт серверы поднятых адаптеров со шлюзом и пропускает туннельные (`dns/transport/local/systemconfig/source_windows.go`).

Таблица «Состояния watchdog и что делать», текст ручного восстановления и виды строк `ABORT` находятся в Global Constraints: этот раздел сопровождает каждую задачу.

---

### Task 1: Подготовка: файлы, правка watchdog, состояние ПК

Задача ничего не меняет в системе: ядро не запускается, Tailscale только спрашивают о состоянии.

**Files:**
- Create: `C:\code\router\spike\plan0\lib.ps1`
- Create: `C:\code\router\spike\plan0\p0lib.py`
- Create: `C:\code\router\spike\plan0\tpl\setup.json`, `C:\code\router\spike\plan0\tpl\hold.json`, `C:\code\router\spike\plan0\tpl\main.json`
- Create: `C:\code\router\spike\plan0\patch_watchdog.py`
- Create: `C:\code\router\spike\plan0\selftest\fake_watchdog.py`, `C:\code\router\spike\plan0\selftest\selftest.py`, `C:\code\router\spike\plan0\selftest\selftest.ps1`
- Create: `C:\code\router\spike\plan0\prepare.py`
- Modify: `C:\code\router\spike\scripts\watchdog.py` (только через `patch_watchdog.py`; исходник сохраняется как `watchdog.py.orig`)

**Interfaces:**
- Consumes: `spike\scripts\watchdog.py` (аргументы `--mode`, `--config`, `--run-id`, `--not-after`; файлы `status.json`, `deadline.txt`, `stop.flag`, `cmd.json` в `spike\run\`), `spike\run\phase-b-v2.json`, `spike\run\clash-secret.txt`, `spike\run\ts-state\`, `spike\run\bin\node-vpn.exe`, `spike\run\bin\node-direct.exe`, `spike\scripts\stun.js`.
- Produces (PowerShell, после `. C:\code\router\spike\plan0\lib.ps1`):
  - `Set-P0Heartbeat` — обновить `deadline.txt`.
  - `Start-P0Watchdog -Mode <hold|engage> -Config <файл в spike\run> [-MaxFails <3..12>]` — запуск с UAC и ожидание; последняя строка `P0-RESULT: ...`.
  - `Send-P0Command -Action <engage|restart|stop> [-Config <файл в spike\run>]` — команда живому watchdog и ожидание; последняя строка `P0-RESULT: ...`.
  - `Get-P0Session` — строка `P0-SESSION: ...` с состоянием и минутами с момента включения; пометка `STALE`, если процесса watchdog уже нет.
  - `Close-P0StaleSession` — закрыть состояние, оставшееся в `status.json` после перезагрузки ПК; последняя строка `P0-RESULT: ...`.
  - `python p0lib.py reset` — вернуть включённое ядро в режим `vpn` и записать базовые файлы правил.
  - `Read-P0Status` — содержимое `spike\run\status.json` объектом; `Format-P0Status <объект>` — одна строка состояния с закрытыми адресами.
- Produces (Python, `from p0lib import *`): `require_state(*states)`, `require_no_session()`, `session_state() -> (state | None, status)`, `heartbeat()`, `lanes() -> (vpn, direct)`, `lane_of(exe, want, vpn, direct) -> (ok, seen)` (при нехватке ответов — `ABORT MEASUREMENT-FAILED`), `wait_lane(exe, want, vpn, direct, limit) -> seconds | None`, `get_mode() -> (mode, mode_list)`, `set_mode(mode) -> (http_status, mode)`, `clash(method, path, body=None) -> (http_status, data)`, `connections() -> list`, `api_cli(args, timeout=30, secret=None) -> (rc, stdout, stderr)`, `ts_status() -> dict`, `ts_exit_nodes() -> (rc, rows)`, `ts_path(peer) -> "direct" | "relay" | "peer-relay" | "none"`, `system_exit_nodes() -> {адрес: в сети}`, `dns_probe(tail) -> dict`, `confirm_pin() -> (ok, why)`, `write_rules(direct, vpn)`, `write_rule_file(name, rules)`, `base_rules() -> (direct, vpn)`, `rule(expression)`, `rule_except(expression, excluded)`, `rx_file(path, upper=False)`, `rx_folder(path, upper=False)`, `euler_circuit(nodes)`, `LogTail`, `whole_log(path)`, `matches_for_process(text, path_pattern)`, `egress_probe(port)`, `egress_self()`, `core_running()`, `system_exit_node_on()`, `local_values()`, `record(check, ok, note, **data)`, `record_live(check, ok, note, _session="ready", **data)` (запись только при живом сеансе, иначе `ABORT SESSION-LOST`); постоянные `COPIES`, `MODES`, `MAIN_LANE`, `DNS_SERVER`, `PIN_NAMES`, `DEST_PIN`, `NODE_VPN`, `NODE_DIRECT`, `STUN_JS`, `RUN`, `OUT`, `RULES`, `EXE`, `CORE_LOG`, `WATCHDOG_LOG`, `SETUP_LOG`, `SETUP_CONFIG`, `SETUP_FRESH_CONFIG`, `FRESH_STATE`, `RESULTS`, `STATUS`, `PROBE_VPN`, `PROBE_DIRECT`, `TERMINAL`, `NO_WINDOW`.
- Produces (файлы): `spike\run\p0-hold.json`, `spike\run\p0-main-vpn.json`, `spike\run\p0-main-vpn-localdns.json`, `spike\run\p0-main-direct.json`, `spike\run\p0-main-direct-localdns.json`, `spike\run\p0-main-alldirect.json`, `spike\run\p0-main-vpn-norevmap.json`, `spike\plan0\out\p0-setup.json`, `spike\plan0\out\p0-setup-fresh.json`, `spike\plan0\rules\lane-direct.json`, `spike\plan0\rules\lane-vpn.json`, копии программ в `spike\plan0\bin\`.
- Produces (watchdog после правки): режим `--mode hold` с состояниями `holding`, `hold_to_engage`, `hold_done`, `hold_failed` (конфиг, адаптер которого не ограничен полем `route_address`, в этом режиме отклоняется: `failed_no_change`); команда `{"action": "engage", "config": "<файл>", "id": "<id>"}`; аргумент `--max-fails <3..12>`; от одной до трёх строк `direct probe before switch:` в `spike\run\watchdog.log` при каждом включении.

- [ ] **Step 1: Показать пользователю ручное восстановление (нужен пользователь: прочитать)**

Шаг выполняет только исполнитель, который сам ведёт разговор с пользователем; подчинённому агенту он не передаётся.

До первого шага с UAC пользователь должен знать, что делать, если связь исполнителя пропала и не вернулась за 10 минут и если компьютер перезагрузился во время проверок. Передайте ему целиком текст из блока «Ручное восстановление» раздела Global Constraints (обе части: про пропавший интернет и про перезагрузку).

Продолжайте после ответа пользователя, что текст прочитан.

- [ ] **Step 2: Создать папки и убедиться, что git их не видит**

```powershell
New-Item -ItemType Directory -Force -Path C:\code\router\spike\plan0\tpl, C:\code\router\spike\plan0\selftest, C:\code\router\spike\plan0\out, C:\code\router\spike\plan0\rules | Out-Null
git -C C:\code\router check-ignore -q spike/plan0/x.txt; if ($LASTEXITCODE -eq 0) { 'IGNORED' } else { 'NOT-IGNORED' }
```

Ожидается: `IGNORED`. При `NOT-IGNORED` остановиться: в `.gitignore` нет строки `spike/`.

- [ ] **Step 3: Записать `lib.ps1`**

Файл `C:\code\router\spike\plan0\lib.ps1`, целиком:

```powershell
# SplitLane Plan 0: heartbeat, launcher, poller and commands for the spike watchdog.
# ASCII only: Windows PowerShell 5.1 reads a script without BOM in the ANSI code page.
# Usage: . C:\code\router\spike\plan0\lib.ps1

$P0Run       = 'C:\code\router\spike\run'
$P0Out       = 'C:\code\router\spike\plan0\out'
$P0Python    = 'C:\Python314\python.exe'
$P0Watchdog  = 'C:\code\router\spike\scripts\watchdog.py'
$P0Tailscale = 'C:\Program Files\Tailscale\tailscale.exe'
$P0Terminal  = @('failed_no_change', 'preflight_ok', 'preflight_failed', 'hold_failed', 'hold_done', 'rolled_back', 'restore_unverified')
$P0FirstStatusSeconds = 60    # how long to wait for the first status of a new run
$P0AfterStopSeconds   = 150   # how long to wait for a terminal state after stop.flag

function Get-P0Now {
    # Never Get-Date -UFormat %s here: on this machine it gives local time with a decimal comma.
    [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
}

function Write-P0Atomic {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)][string]$Text)
    # No BOM, temp file in the same folder, then rename over the target.
    $tmp = "$Path.pstmp"
    [IO.File]::WriteAllText($tmp, $Text, (New-Object Text.UTF8Encoding $false))
    foreach ($attempt in 1..20) {
        try { Move-Item -LiteralPath $tmp -Destination $Path -Force -ErrorAction Stop; return }
        catch { Start-Sleep -Milliseconds 100 }   # the reader had the target open; try again
    }
    throw "Write-P0Atomic: could not replace $Path"
}

function Set-P0Heartbeat {
    # Every command starts with this while a core is up: deadline = now + 400 s.
    Write-P0Atomic -Path (Join-Path $P0Run 'deadline.txt') -Text ([string]((Get-P0Now) + 400))
}

function Read-P0Json {
    param([Parameter(Mandatory)][string]$Path)
    try { Get-Content -LiteralPath $Path -Raw -ErrorAction Stop | ConvertFrom-Json } catch { $null }
}

function Read-P0Status { Read-P0Json -Path (Join-Path $P0Run 'status.json') }

function Format-P0Status {
    param($Status)
    if (-not $Status) { return 'no status' }
    $text = "state=$($Status.state) run=$($Status.run) seq=$($Status.seq) cmd_id=$($Status.cmd_id)"
    foreach ($name in 'config', 'reason', 'cmd_error', 'system_vpn_ok', 'exit_node_still_on') {
        if ($null -ne $Status.$name) { $text += " $name=$($Status.$name)" }
    }
    # Addresses never reach the console.
    $text -replace '\b\d{1,3}(\.\d{1,3}){3}\b', '<ip>'
}

function Test-P0WatchdogGone {
    # True when no python.exe that could have written this status is running. The watchdog process
    # is always older than its own status, so "no older python.exe" means it is gone (the PC was
    # restarted, or the process was ended). Any doubt gives False.
    # A python.exe whose command line is readable and does not name watchdog.py is somebody else's;
    # an elevated one has no readable command line and still counts.
    param($Status)
    if (-not $Status -or -not $Status.time) { return $false }
    try {
        $written = [DateTimeOffset]::FromUnixTimeSeconds([int64]$Status.time).LocalDateTime.AddSeconds(2)
        $all = @(Get-CimInstance -ClassName Win32_Process -Filter "Name='python.exe'" -ErrorAction Stop)
    } catch { return $false }
    @($all | Where-Object { $_.CreationDate -le $written -and (-not $_.CommandLine -or $_.CommandLine -like '*watchdog.py*') }).Count -eq 0
}

function Wait-P0Status {
    # Polls status.json for a target or terminal state of OUR run (and OUR command).
    # On timeout it writes stop.flag, so that a watchdog we lost track of rolls back.
    param([Parameter(Mandatory)][string]$RunId, [Parameter(Mandatory)][string[]]$Target,
          [string]$CmdId = '', [int]$TimeoutSeconds = 300)
    $started = Get-Date
    $limit = $started.AddSeconds($TimeoutSeconds)
    $last = ''
    while ((Get-Date) -lt $limit) {
        $s = Read-P0Status
        if ($s -and $s.run -eq $RunId) {
            $line = Format-P0Status $s
            if ($line -ne $last) { Write-Host "P0-STATUS: $line"; $last = $line }
            $mine = (-not $CmdId) -or ($s.cmd_id -eq $CmdId)
            if ($CmdId -and $mine -and $s.cmd_error) { return $s }
            if ($mine -and ($Target -contains $s.state)) { return $s }
            if ($P0Terminal -contains $s.state) { return $s }
        } elseif (-not $last -and ((Get-Date) - $started).TotalSeconds -gt $P0FirstStatusSeconds) {
            break   # the watchdog writes its first status within a second of starting
        }
        Start-Sleep -Milliseconds 500
    }
    Write-Host 'P0-STATUS: POLL-TIMEOUT - writing stop.flag'
    [IO.File]::WriteAllText((Join-Path $P0Run 'stop.flag'), 'poll timeout', (New-Object Text.ASCIIEncoding))
    $limit = (Get-Date).AddSeconds($P0AfterStopSeconds)
    while ((Get-Date) -lt $limit) {
        $s = Read-P0Status
        if ($s -and $s.run -eq $RunId -and ($P0Terminal -contains $s.state)) { return $s }
        Start-Sleep -Milliseconds 500
    }
    $s = Read-P0Status
    if ($s -and $s.run -eq $RunId) {
        Write-Host 'P0-STATUS: NO-TERMINAL-STATE after stop.flag - ask the user to do the manual recovery'
        return $s
    }
    Write-Host 'P0-STATUS: NO-STATUS - the watchdog of this run never reported; look for a leftover python.exe or sing-box.exe'
    $null
}

function Start-P0Watchdog {
    # Elevated launch (UAC prompt) and wait for the result, all in one call.
    param([Parameter(Mandatory)][ValidateSet('hold', 'engage')][string]$Mode,
          [Parameter(Mandatory)][string]$Config,
          [ValidateRange(3, 12)][int]$MaxFails = 3)
    $old = Read-P0Status
    if ($old -and ($P0Terminal -notcontains $old.state)) {
        # No override here: a status left behind by a reboot is closed with Close-P0StaleSession.
        Write-Host "P0-RESULT: REFUSED - the previous watchdog has not finished: $(Format-P0Status $old)"
        return
    }
    if (Get-Process -Name 'sing-box' -ErrorAction SilentlyContinue) {
        Write-Host 'P0-RESULT: REFUSED - a sing-box.exe is running; stop it first'
        return
    }
    if (-not (Test-Path -LiteralPath (Join-Path $P0Run $Config))) {
        Write-Host "P0-RESULT: REFUSED - config not found in run\: $Config"
        return
    }
    $ts = try { (& $P0Tailscale status --json | Out-String | ConvertFrom-Json) } catch { $null }
    if (-not $ts -or -not $ts.ExitNodeStatus) {
        Write-Host 'P0-RESULT: REFUSED - the system exit node is not on'
        return
    }
    $runId = [guid]::NewGuid().ToString('N').Substring(0, 8)
    $notAfter = (Get-P0Now) + 120
    $argList = @($P0Watchdog, '--mode', $Mode, '--config', $Config, '--run-id', $runId,
                 '--not-after', "$notAfter", '--max-fails', "$MaxFails")
    Write-Host "P0-LAUNCH: run=$runId mode=$Mode config=$Config - the UAC prompt must be accepted within 120 s"
    try {
        Start-Process -FilePath $P0Python -ArgumentList $argList -Verb RunAs -WindowStyle Hidden -ErrorAction Stop | Out-Null
    } catch {
        Write-Host 'P0-RESULT: UAC-CANCELLED - nothing was started, the system is unchanged. Do not retry without the user.'
        return
    }
    $session = [ordered]@{ run = $runId; mode = $Mode; started = (Get-P0Now); engaged = 0 }
    Write-P0Atomic -Path (Join-Path $P0Out 'session.json') -Text ($session | ConvertTo-Json -Compress)
    $target = @{ hold = 'holding'; engage = 'ready' }[$Mode]
    $s = Wait-P0Status -RunId $runId -Target @($target) -TimeoutSeconds 300
    if ($s -and $s.state -eq 'ready') {
        $session.engaged = Get-P0Now
        Write-P0Atomic -Path (Join-Path $P0Out 'session.json') -Text ($session | ConvertTo-Json -Compress)
        Set-P0Heartbeat
    }
    if ($s -and $s.state -eq 'holding') { Set-P0Heartbeat }   # the watchdog itself grants only 300 s
    Write-Host "P0-RESULT: $(Format-P0Status $s)"
}

function Send-P0Command {
    # engage (from hold), restart (new config), stop (roll back). Waits for the outcome of OUR command id.
    param([Parameter(Mandatory)][ValidateSet('engage', 'restart', 'stop')][string]$Action, [string]$Config = '')
    $session = Read-P0Json -Path (Join-Path $P0Out 'session.json')
    $s = Read-P0Status
    if (-not $session -or -not $s -or $s.run -ne $session.run -or ($P0Terminal -contains $s.state)) {
        Write-Host "P0-RESULT: NO-LIVE-SESSION - $(Format-P0Status $s)"
        return
    }
    if (Test-P0WatchdogGone $s) {
        # Nobody would answer the command. A stop still leaves stop.flag behind in case this test is wrong:
        # a live watchdog rolls back on it, and the next watchdog removes a leftover flag when it starts.
        if ($Action -eq 'stop') {
            [IO.File]::WriteAllText((Join-Path $P0Run 'stop.flag'), 'stop on a stale status', (New-Object Text.ASCIIEncoding))
        }
        Write-Host "P0-RESULT: NO-LIVE-SESSION - stale status, the watchdog process is gone: $(Format-P0Status $s)"
        return
    }
    if ($Action -ne 'stop' -and -not (Test-Path -LiteralPath (Join-Path $P0Run $Config))) {
        Write-Host "P0-RESULT: REFUSED - config not found in run\: $Config"
        return
    }
    $id = 'c' + [guid]::NewGuid().ToString('N').Substring(0, 7)
    $cmd = [ordered]@{ action = $Action; id = $id }
    if ($Action -eq 'stop') { $cmd.hard = $true } else { $cmd.config = $Config }
    Set-P0Heartbeat
    Write-P0Atomic -Path (Join-Path $P0Run 'cmd.json') -Text ($cmd | ConvertTo-Json -Compress)
    Write-Host "P0-COMMAND: $Action $Config id=$id"
    $target = if ($Action -eq 'stop') { @('rolled_back', 'hold_done') } else { @('ready') }
    $r = Wait-P0Status -RunId $session.run -Target $target -CmdId $id -TimeoutSeconds 300
    if ($Action -ne 'stop' -and $r -and $r.state -eq 'ready' -and -not $r.cmd_error) {
        if (-not $session.engaged) {
            $new = [ordered]@{ run = $session.run; mode = $session.mode; started = $session.started; engaged = (Get-P0Now) }
            Write-P0Atomic -Path (Join-Path $P0Out 'session.json') -Text ($new | ConvertTo-Json -Compress)
        }
        Set-P0Heartbeat
    }
    Write-Host "P0-RESULT: $(Format-P0Status $r)"
}

function Get-P0Session {
    # One line: state of our watchdog and minutes since the core was engaged (the watchdog rolls back at 45).
    $session = Read-P0Json -Path (Join-Path $P0Out 'session.json')
    $s = Read-P0Status
    if (-not $session -or -not $s -or $s.run -ne $session.run) { Write-Host 'P0-SESSION: none'; return }
    $minutes = 0
    if ($session.engaged) { $minutes = [int][math]::Floor(((Get-P0Now) - $session.engaged) / 60) }
    $stale = ''
    if (($P0Terminal -notcontains $s.state) -and (Test-P0WatchdogGone $s)) {
        $stale = ' STALE - the watchdog process is gone, see Close-P0StaleSession'
    }
    Write-Host "P0-SESSION: $(Format-P0Status $s) engaged_minutes=$minutes (hard cap 45)$stale"
}

function Close-P0StaleSession {
    # After a reboot (or an ended watchdog process) status.json still says holding or ready, but nobody
    # is there. Marks the session as ended WITHOUT a verified restore, so that the state of the system
    # is checked next (prepare.py). Refuses while a core or a possible watchdog process is alive.
    $s = Read-P0Status
    if (-not $s -or ($P0Terminal -contains $s.state)) {
        Write-Host "P0-RESULT: NOTHING-TO-CLOSE - $(Format-P0Status $s)"
        return
    }
    if (Get-Process -Name 'sing-box' -ErrorAction SilentlyContinue) {
        Write-Host 'P0-RESULT: REFUSED - a sing-box.exe is running without a watchdog; the user does the manual recovery first'
        return
    }
    if (-not (Test-P0WatchdogGone $s)) {
        Write-Host 'P0-RESULT: REFUSED - a python.exe older than the last status is running; it may be the watchdog'
        return
    }
    $closed = [ordered]@{
        run = $s.run; seq = ([int]$s.seq + 1); state = 'restore_unverified'; cmd_id = $s.cmd_id; time = (Get-P0Now)
        reason = "stale status closed, the watchdog process is gone (was $($s.state))"; system_vpn_ok = $false
    }
    Write-P0Atomic -Path (Join-Path $P0Run 'status.json') -Text ($closed | ConvertTo-Json -Compress)
    Write-Host "P0-RESULT: $(Format-P0Status (Read-P0Status))"
}
```

- [ ] **Step 4: Записать `p0lib.py`**

Файл `C:\code\router\spike\plan0\p0lib.py`, целиком:

```python
"""SplitLane Plan 0: shared helpers for the on-PC checks.

Throwaway code, lives in spike/plan0 (ignored by git). It reads the real
addresses and secrets from the local ignored files at run time and never prints
them: the output uses the lane names VPN / DIRECT instead of addresses.
"""
import json
import os
import re
import secrets
import shutil
import socket
import struct
import subprocess
import sys
import time
import urllib.error
import urllib.request

SPIKE = r"C:\code\router\spike"
RUN = os.path.join(SPIKE, "run")
P0 = os.path.join(SPIKE, "plan0")
OUT = os.path.join(P0, "out")
TPL = os.path.join(P0, "tpl")
RULES = os.path.join(P0, "rules")
BIN = os.path.join(P0, "bin")
EXE = os.path.join(SPIKE, "dl", "sing-box-1.14.4", "sing-box-1.14.4-windows-amd64", "sing-box.exe")
WATCHDOG = os.path.join(SPIKE, "scripts", "watchdog.py")
WATCHDOG_LOG = os.path.join(RUN, "watchdog.log")
PROVEN = os.path.join(RUN, "phase-b-v2.json")
CORE_LOG = os.path.join(RUN, "phase-b.log")
STATUS = os.path.join(RUN, "status.json")
DEADLINE = os.path.join(RUN, "deadline.txt")
SESSION = os.path.join(OUT, "session.json")
RESULTS = os.path.join(OUT, "results.json")
LANES = os.path.join(OUT, "lanes.json")
SETUP_LOG = os.path.join(OUT, "setup.log")
SETUP_CONFIG = os.path.join(OUT, "p0-setup.json")
SETUP_FRESH_CONFIG = os.path.join(OUT, "p0-setup-fresh.json")
FRESH_STATE = os.path.join(OUT, "ts-fresh")
API_SECRET_FILE = os.path.join(P0, "api-secret.txt")
TS = r"C:\Program Files\Tailscale\tailscale.exe"
SYSTEM_CURL = r"C:\Windows\System32\curl.exe"
NODE_VPN = os.path.join(RUN, "bin", "node-vpn.exe")
NODE_DIRECT = os.path.join(RUN, "bin", "node-direct.exe")
STUN_JS = os.path.join(SPIKE, "scripts", "stun.js")

CLASH = "http://127.0.0.1:19090"
API_ADDR = "127.0.0.1:19091"
PROBE_VPN = 17891
PROBE_DIRECT = 17892
TUN_DNS = "172.19.0.2"
IP_SERVICE = "https://api.ipify.org"
MODES = ["vpn", "vpn-localdns", "direct", "direct-localdns", "alldirect"]
MAIN_LANE = {"vpn": "VPN", "vpn-localdns": "VPN", "direct": "DIRECT", "direct-localdns": "DIRECT",
             "alldirect": "DIRECT"}
DNS_SERVER = {"vpn": "final", "vpn-localdns": "dns-local", "direct": "final", "direct-localdns": "dns-local",
              "alldirect": "dns-local"}
PIN_NAMES = ["claude.exe", "bun.exe", "node.exe", "python.exe", "pythonw.exe"]
DEST_PIN = "160.79.104.0/23"  # published address range of the API the executor talks to; rule 9 pins it to the VPN lane
TERMINAL = ["failed_no_change", "preflight_ok", "preflight_failed", "hold_failed", "hold_done", "rolled_back",
            "restore_unverified"]
NO_WINDOW = 0x08000000

COPIES = {
    "free": os.path.join(BIN, "free", "curl-free.exe"),
    "pin-vpn": os.path.join(BIN, "pin", "curl-pin-vpn.exe"),
    "pin-direct": os.path.join(BIN, "pin", "curl-pin-direct.exe"),
    "space": os.path.join(BIN, "Dir With Spaces", "curl-space.exe"),
    "brackets": os.path.join(BIN, "Brackets (x86) [v1]", "curl-brackets.exe"),
    "cyr": os.path.join(BIN, "Кириллица Папка", "curl-кириллица.exe"),
    "outer": os.path.join(BIN, "nest", "outer", "curl-outer.exe"),
    "inner": os.path.join(BIN, "nest", "outer", "inner", "curl-inner.exe"),
    "inline": os.path.join(BIN, "inline", "curl-inline.exe"),
}

_values = None

# The output is ASCII by design; this only keeps an unexpected character from raising.
sys.stdout.reconfigure(errors="backslashreplace")


def die(message):
    print("ABORT " + message)
    sys.exit(2)


def read_json(path):
    try:
        with open(path, "r", encoding="utf-8-sig") as f:
            return json.load(f)
    except (OSError, ValueError):
        return None


def write_atomic(path, text):
    """UTF-8 without BOM: temp file in the same folder, then rename (retried, the core may be reading)."""
    tmp = path + ".pytmp"
    with open(tmp, "w", encoding="utf-8", newline="\n") as f:
        f.write(text)
    for _ in range(20):
        try:
            os.replace(tmp, path)
            return
        except PermissionError:
            time.sleep(0.1)
    os.replace(tmp, path)


def local_values():
    """Real values from the ignored spike files. Never print the result."""
    global _values
    if _values is not None:
        return _values
    endpoint = read_json(PROVEN)["endpoints"][0]
    with open(WATCHDOG, "r", encoding="utf-8") as f:
        source = f.read()
    vpn_ip = re.search(r'^EXPECT_IP\s*=\s*"([^"]+)"', source, re.M).group(1)
    exit_node = re.search(r'^EXIT_NODE\s*=\s*"([^"]+)"', source, re.M).group(1)
    if exit_node != endpoint["exit_node"]:
        die("exit node in watchdog.py differs from the one in phase-b-v2.json")
    with open(os.path.join(RUN, "clash-secret.txt"), "r", encoding="utf-8-sig") as f:
        clash_secret = f.read().strip()
    if not os.path.exists(API_SECRET_FILE):
        write_atomic(API_SECRET_FILE, secrets.token_hex(16))
    with open(API_SECRET_FILE, "r", encoding="utf-8") as f:
        api_secret = f.read().strip()
    _values = {"exit_node": exit_node, "hostname": endpoint["hostname"], "vpn_ip": vpn_ip,
               "clash_secret": clash_secret, "api_secret": api_secret}
    return _values


# ---- configs -------------------------------------------------------------

def check_config(path):
    p = subprocess.run([EXE, "check", "-c", path, "-D", RUN], capture_output=True, timeout=60,
                       creationflags=NO_WINDOW)
    return p.returncode == 0, (p.stdout + p.stderr).decode("utf-8", "replace").strip()[:400]


def render(template, out_path, default_mode="vpn", reverse_mapping=True, state_dir=None, hostname=None):
    """Template with tokens -> real config (BOM-less, atomic) -> sing-box check. Returns (ok, message)."""
    v = local_values()
    with open(os.path.join(TPL, template), "r", encoding="utf-8") as f:
        text = f.read()
    tokens = {
        "__EXIT_NODE__": v["exit_node"],
        "__TS_HOSTNAME__": hostname or v["hostname"],
        "__TS_STATE_DIR__": (state_dir or os.path.join(RUN, "ts-state")).replace("\\", "/"),
        "__CLASH_SECRET__": v["clash_secret"],
        "__API_SECRET__": v["api_secret"],
        "__DEFAULT_MODE__": default_mode,
    }
    for token, value in tokens.items():
        text = text.replace(token, value)
    left = sorted(set(re.findall(r"__[A-Z_]+__", text)))
    if left:
        return False, "unreplaced tokens: %s" % left
    config = json.loads(text)
    if not reverse_mapping:
        config["dns"]["reverse_mapping"] = False
    write_atomic(out_path, json.dumps(config, indent=2) + "\n")
    return check_config(out_path)


# ---- rule files ----------------------------------------------------------

def quote_meta(text):
    """Same character set as Go regexp.QuoteMeta."""
    return re.sub(r"([\\.+*?()|\[\]{}^$])", r"\\\1", text)


def rx_file(path, upper=False):
    body = quote_meta(path)
    return "(?i)^" + (body.upper() if upper else body) + "$"


def rx_folder(path, upper=False):
    body = quote_meta(path.rstrip("\\") + "\\")
    return "(?i)^" + (body.upper() if upper else body)


STUB = {"process_path_regex": ["^:splitlane-stub$"]}


def rule(expression):
    return {"process_path_regex": [expression]}


def rule_except(expression, excluded):
    """Spec 8.2: the path matches this rule AND does not match the nested rules of the other lane."""
    return {"type": "logical", "mode": "and",
            "rules": [rule(expression)] + [dict(rule(x), invert=True) for x in excluded]}


def write_rule_file(name, rules):
    document = {"version": 3, "rules": [STUB] + rules}
    write_atomic(os.path.join(RULES, name), json.dumps(document, ensure_ascii=False, indent=2) + "\n")


def write_rules(direct, vpn):
    write_rule_file("lane-direct.json", direct)
    write_rule_file("lane-vpn.json", vpn)


def base_rules():
    direct = [rule(rx_file(COPIES["pin-direct"])), rule(rx_file(NODE_DIRECT))]
    vpn = [rule(rx_file(COPIES["pin-vpn"])), rule(rx_file(NODE_VPN))]
    return direct, vpn


def ensure_copies():
    """Renamed copies of the system curl.exe: the only programs ever put into a direct rule."""
    for path in COPIES.values():
        os.makedirs(os.path.dirname(path), exist_ok=True)
        if not os.path.exists(path):
            shutil.copy2(SYSTEM_CURL, path)
    for path in (NODE_VPN, NODE_DIRECT):
        if not os.path.exists(path):
            node = shutil.which("node")
            if not node:
                die("node.exe not found and %s is missing" % os.path.basename(path))
            shutil.copy2(node, path)


# ---- watchdog session ----------------------------------------------------

def heartbeat():
    """Rule: every command refreshes the deadline while a core is up (now + 400 s, ASCII)."""
    tmp = DEADLINE + ".pytmp"
    with open(tmp, "w", encoding="ascii") as f:
        f.write(str(int(time.time()) + 400))
    for _ in range(20):
        try:
            os.replace(tmp, DEADLINE)
            return
        except PermissionError:
            time.sleep(0.1)


def core_running():
    out = subprocess.run(["tasklist", "/FI", "IMAGENAME eq sing-box.exe", "/FO", "CSV", "/NH"],
                         capture_output=True, timeout=15, creationflags=NO_WINDOW).stdout
    return b"sing-box.exe" in out.lower()


def system_exit_node_on():
    """Read-only question to the system Tailscale client."""
    p = subprocess.run([TS, "status", "--json"], capture_output=True, timeout=20, creationflags=NO_WINDOW)
    try:
        return bool(json.loads(p.stdout.decode("utf-8", "replace")).get("ExitNodeStatus"))
    except ValueError:
        return False


def system_exit_nodes():
    """{tailnet IPv4: online} of the devices the system client lists as exit node options (read-only)."""
    p = subprocess.run([TS, "status", "--json"], capture_output=True, timeout=20, creationflags=NO_WINDOW)
    try:
        status = json.loads(p.stdout.decode("utf-8", "replace"))
    except ValueError:
        die("MEASUREMENT-FAILED: the system Tailscale client did not answer; nothing recorded, repeat the step")
    found = {}
    for peer in [status.get("Self") or {}] + list((status.get("Peer") or {}).values()):
        if peer.get("ExitNodeOption"):
            for ip in peer.get("TailscaleIPs") or []:
                if "." in ip:
                    found[ip] = bool(peer.get("Online"))
    return found


def session_state():
    status = read_json(STATUS) or {}
    session = read_json(SESSION) or {}
    if not session.get("run") or status.get("run") != session.get("run"):
        return None, status
    return status.get("state"), status


def require_state(*states):
    """Abort unless the watchdog of this session reports one of the given states and its core is running."""
    state, status = session_state()
    if state not in states:
        die("SESSION-NOT-READY: wanted %s, status.json says %s" % ("/".join(states), state))
    if not core_running():
        die("SESSION-NOT-READY: status.json says %s, but no sing-box.exe is running (a stale status after a reboot?)"
            % state)
    heartbeat()
    return status


def require_no_session():
    """For the checks that run their own, not elevated core."""
    status = read_json(STATUS) or {}
    if status.get("state") not in TERMINAL:
        die("a watchdog session is live (state %s); stop it first" % status.get("state"))


# ---- HTTP, Clash API, API service ---------------------------------------

_no_proxy = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def http(method, url, body=None, headers=None, timeout=8, opener=None):
    data = json.dumps(body).encode("utf-8") if body is not None else None
    request = urllib.request.Request(url, data=data, method=method, headers=headers or {})
    if data is not None:
        request.add_header("Content-Type", "application/json")
    try:
        with (opener or _no_proxy).open(request, timeout=timeout) as response:
            return response.status, response.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, ""
    except Exception as e:
        return None, repr(e)[:160]


def clash(method, path, body=None):
    status, text = http(method, CLASH + path, body, {"Authorization": "Bearer " + local_values()["clash_secret"]})
    try:
        return status, json.loads(text) if text else None
    except ValueError:
        return status, None


def get_mode():
    _, data = clash("GET", "/configs")
    data = data or {}
    return data.get("mode"), data.get("mode-list") or []


def set_mode(mode):
    """PATCH /configs, then read back. Returns (http status of the PATCH, mode after it)."""
    status, _ = clash("PATCH", "/configs", {"mode": mode})
    return status, get_mode()[0]


def connections():
    _, data = clash("GET", "/connections")
    return (data or {}).get("connections") or []


def api_cli(args, timeout=30, secret=None):
    """sing-box api <args> against the API service; URL and secret go through the environment."""
    env = dict(os.environ, BOX_API_URL=API_ADDR,
               BOX_API_SECRET=local_values()["api_secret"] if secret is None else secret)
    try:
        p = subprocess.run([EXE, "api"] + list(args), capture_output=True, timeout=timeout, env=env,
                           creationflags=NO_WINDOW)
    except subprocess.TimeoutExpired:
        return -1, "", "timeout"
    return p.returncode, p.stdout.decode("utf-8", "replace"), p.stderr.decode("utf-8", "replace")


def parse_ts_status(out):
    info = {}
    for line in out.splitlines():
        m = re.match(r"^\s+([A-Za-z ]+):\s+(.*)$", line)
        if m:
            info[m.group(1).strip()] = m.group(2).strip()
    return info


def ts_status():
    """Fields of `api tailscale status`: State, Network, Auth, Log in."""
    rc, out, err = api_cli(["tailscale", "status"])
    info = parse_ts_status(out)
    info["rc"] = rc
    return info


def parse_exit_nodes(out):
    rows = []
    for line in out.splitlines():
        cells = line.split("\t")
        if len(cells) == 4:
            rows.append({"name": cells[0], "ip": cells[1], "online": cells[2], "status": cells[3]})
    return rows


def ts_exit_nodes():
    """Rows of `api tailscale exit-node list` (tab separated when stdout is not a terminal)."""
    rc, out, err = api_cli(["tailscale", "exit-node", "list"])
    return rc, parse_exit_nodes(out)


def parse_path(out):
    pongs = [line for line in out.splitlines() if line.startswith("pong from") and " via " in line]
    if not pongs:
        return "none"
    via = pongs[-1].split(" via ", 1)[1]
    if via.startswith("DERP("):
        return "relay"
    if via.startswith("peer relay"):
        return "peer-relay"
    return "direct"


def ts_path(peer):
    """direct / relay / peer-relay / none, from `api tailscale ping`."""
    rc, out, err = api_cli(["tailscale", "ping", peer], timeout=75)
    return parse_path(out)


# ---- lanes ---------------------------------------------------------------

def is_ipv4(text):
    return bool(re.fullmatch(r"\d{1,3}(\.\d{1,3}){3}", text or ""))


def egress_probe(port, timeout=8):
    proxy = "http://127.0.0.1:%d" % port
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({"http": proxy, "https": proxy}))
    status, text = http("GET", IP_SERVICE, opener=opener, timeout=timeout)
    return text.strip() if status == 200 and is_ipv4(text.strip()) else None


def egress_self(timeout=8):
    status, text = http("GET", IP_SERVICE, timeout=timeout)
    return text.strip() if status == 200 and is_ipv4(text.strip()) else None


def egress_exe(exe, timeout=10):
    try:
        p = subprocess.run([exe, "-4", "-s", "--noproxy", "*", "--max-time", str(timeout), IP_SERVICE],
                           capture_output=True, timeout=timeout + 5, creationflags=NO_WINDOW)
    except Exception:
        return None
    text = p.stdout.decode("ascii", "replace").strip()
    return text if is_ipv4(text) else None


def lanes():
    """(vpn address, direct address) measured through the two probe inbounds."""
    vpn = egress_probe(PROBE_VPN) or egress_probe(PROBE_VPN)
    direct = egress_probe(PROBE_DIRECT) or egress_probe(PROBE_DIRECT)
    if not vpn or not direct:
        die("lane addresses not measurable: probe-vpn=%s probe-direct=%s" % (bool(vpn), bool(direct)))
    if vpn == direct:
        die("probe-vpn and probe-direct leave through the same address")
    if vpn != local_values()["vpn_ip"]:
        die("probe-vpn does not leave through the expected exit node address")
    write_atomic(LANES, json.dumps({"vpn": vpn, "direct": direct}))  # local only, for the privacy scan
    return vpn, direct


def lane_name(ip, vpn, direct):
    if ip is None:
        return "NONE"
    return "VPN" if ip == vpn else "DIRECT" if ip == direct else "OTHER"


def lane_of(exe, want, vpn, direct):
    """Verdict on ANSWERS only: ok when the first answer is right or two of three answers are.

    A request without an answer is repeated (six requests at most); too few answers stop the script,
    because a failed measurement is not a property of the core. Returns (ok, seen).
    """
    seen, answers = [], []
    while len(seen) < 6:
        lane = lane_name(egress_exe(exe), vpn, direct)
        seen.append(lane)
        if lane == "NONE":
            continue
        answers.append(lane)
        if answers[0] == want or len(answers) == 3:
            break
    if answers and answers[0] == want:
        return True, seen
    if len(answers) < 3:
        die("MEASUREMENT-FAILED: %d answers in %d requests (%s); nothing recorded, repeat the step" % (
            len(answers), len(seen), seen))
    return answers.count(want) >= 2, seen


def wait_lane(exe, want, vpn, direct, limit=5.0):
    """Seconds from the call to the START of the first request that left through the wanted lane, or None.

    The request itself (process start, TLS, answer: 0.3-1.5 s) is not part of the reload time.
    """
    start = time.time()
    while time.time() - start < limit:
        began = time.time()
        if lane_name(egress_exe(exe, timeout=4), vpn, direct) == want:
            return round(began - start, 1)
        time.sleep(0.25)
    return None


def euler_circuit(nodes):
    """Every ordered pair (a, b), a != b, exactly once; starts and ends at nodes[0]."""
    edges = {a: [b for b in nodes if b != a] for a in nodes}
    stack, circuit = [nodes[0]], []
    while stack:
        top = stack[-1]
        if edges[top]:
            stack.append(edges[top].pop(0))
        else:
            circuit.append(stack.pop())
    return circuit[::-1]


# ---- core log ------------------------------------------------------------

class LogTail:
    """Reads what the core appended to its log after mark()."""

    def __init__(self, path=CORE_LOG):
        self.path = path
        self.pos = 0
        self.mark()

    def mark(self):
        try:
            self.pos = os.path.getsize(self.path)
        except OSError:
            self.pos = 0

    def text(self):
        try:
            if os.path.getsize(self.path) < self.pos:
                self.pos = 0  # a core restart replaced the log
            with open(self.path, "rb") as f:
                f.seek(self.pos)
                return f.read().decode("utf-8", "replace")
        except OSError:
            return ""

    def wait(self, pattern, timeout=6.0):
        end = time.time() + timeout
        while True:
            m = re.search(pattern, self.text(), re.M)
            if m or time.time() > end:
                return m
            time.sleep(0.2)


def whole_log(path=CORE_LOG):
    try:
        with open(path, "rb") as f:
            return f.read().decode("utf-8", "replace")
    except OSError:
        return ""


def matches_for_process(text, path_pattern):
    """[(connection id, rule index, rule text, action)] for connections whose owner path matches.

    Action is the text after "=>", for example "route(vpn)", "reject", "hijack-dns". Rule index is
    None when no rule matched, that is the connection took the final outbound. A connection logged
    in the last few hundred bytes is skipped: its match line may not be written yet.
    """
    found = []
    for m in re.finditer(r"\[(\d+) [^\]]*\] router: found process path: (.*)", text):
        if not re.search(path_pattern, m.group(2).strip(), re.I) or m.end() > len(text) - 300:
            continue
        nearby = text[m.end():m.end() + 20000]
        hit = re.search(r"\[%s [^\]]*\] router: (?:pre-)?match\[(\d+)\] (.*) => (.*)" % m.group(1), nearby)
        if hit:
            found.append((m.group(1), int(hit.group(1)), hit.group(2), hit.group(3).strip()))
        else:
            found.append((m.group(1), None, "", ""))
    return found


def dns_query(name, timeout=5.0):
    """One A query sent straight to the adapter's DNS address, so the Windows DNS cache is not involved."""
    packet = struct.pack(">HHHHHH", secrets.randbits(16), 0x0100, 1, 0, 0, 0)
    packet += b"".join(bytes([len(label)]) + label.encode("ascii") for label in name.split("."))
    packet += bytes([0]) + struct.pack(">HH", 1, 1)
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(timeout)
    try:
        sock.sendto(packet, (TUN_DNS, 53))
        data, _ = sock.recvfrom(2048)
    except OSError:
        return None
    finally:
        sock.close()
    if len(data) < 12 or data[:2] != packet[:2]:
        return None
    flags, _, answers = struct.unpack(">HHH", data[2:8])
    return {"rcode": flags & 0x000F, "answers": answers}


def dns_probe(tail, name="www.example.com"):
    """Ask the core for a name and report which of its DNS rules handled the query."""
    tail.mark()
    reply = dns_query(name)
    answered = bool(reply) and reply["rcode"] == 0 and reply["answers"] > 0
    pattern = r"\[(\d+) [^\]]*\] dns: exchange %s\. IN A" % re.escape(name)
    if not tail.wait(pattern, timeout=5):
        return {"seen": False, "answered": answered, "server": "?"}
    time.sleep(0.3)
    text = tail.text()
    query_id = re.findall(pattern, text)[-1]
    hit = re.search(r"\[%s [^\]]*\] dns: match\[\d+\] (.*) => route\(([^)]*)\)" % query_id, text)
    return {"seen": True, "answered": answered, "server": hit.group(2) if hit else "final"}


def confirm_pin():
    """Safety gate before any mode whose main lane is direct.

    Static: the rule that pins the executor's processes to the VPN outbound
    and the rule that pins the executor's API addresses to it (for connections
    whose owner the core did not find) both precede every clash_mode rule.
    Dynamic: a request of this python.exe (it is in the process rule) is
    matched by exactly that rule in the core log, and so is every claude.exe /
    bun.exe connection the core has logged so far.
    """
    status = read_json(STATUS) or {}
    config = read_json(os.path.join(RUN, str(status.get("config", "")))) or {}
    rules = (config.get("route") or {}).get("rules") or []
    pin = next((i for i, r in enumerate(rules) if "claude.exe" in (r.get("process_name") or [])), None)
    first_mode = next((i for i, r in enumerate(rules) if "clash_mode" in r), None)
    if pin is None or first_mode is None or pin > first_mode or rules[pin].get("outbound") != "vpn":
        return False, "static: pin rule missing or not before the mode rules"
    missing = [n for n in PIN_NAMES if n not in rules[pin]["process_name"]]
    if missing:
        return False, "static: pin rule lacks %s" % missing
    dest = next((i for i, r in enumerate(rules)
                 if DEST_PIN in (r.get("ip_cidr") or []) and r.get("outbound") == "vpn"), None)
    if dest is None or dest > first_mode:
        return False, "static: destination pin rule missing or not before the mode rules"
    tail = LogTail()
    egress_self()
    time.sleep(0.5)
    own = []
    for _ in range(25):
        own = matches_for_process(tail.text() + " " * 300, re.escape(sys.executable) + "$")
        if any(index == pin and action == "route(vpn)" for _, index, _, action in own):
            break
        time.sleep(0.2)
    else:
        return False, "dynamic: python.exe request not matched by rule %d" % pin
    # Earlier rules (IPv6 reject, DNS hijack, private addresses) are fine; a later rule or none is not.
    seen = own + matches_for_process(whole_log(), r"\\(claude|bun)\.exe$")
    bad = [x for x in seen if x[1] is None or x[1] > pin]
    if bad:
        return False, "dynamic: %d connections of pinned programs passed the pin rule %d" % (len(bad), pin)
    return True, "pin rule %d precedes mode rule %d; python.exe matched it; pinned connections checked: %d" % (
        pin, first_mode, len(seen))


# ---- results -------------------------------------------------------------

def record(check, ok, note="", **data):
    """ok: True = PASS, False = FAIL, None = INFO. Data must not contain addresses or names."""
    os.makedirs(OUT, exist_ok=True)
    results = read_json(RESULTS) or {}
    results[check] = {"ok": ok, "note": note, "time": time.strftime("%Y-%m-%d %H:%M:%S"), "data": data}
    write_atomic(RESULTS, json.dumps(results, indent=2, ensure_ascii=True) + "\n")
    print("%s %s %s" % ({True: "PASS", False: "FAIL", None: "INFO"}[ok], check, note))
    return ok


def record_live(check, ok, note="", _session="ready", **data):
    """record() for checks that need the watchdog session: no verdict once the session left <_session>.

    A rollback while a script runs is an event of the harness. Whatever was measured around it says
    nothing about the core, so nothing is recorded and the script stops.
    """
    state, status = session_state()
    if state != _session:
        reason = re.sub(r"\d{1,3}(\.\d{1,3}){3}", "<ip>", str(status.get("reason")))
        die("SESSION-LOST before %s was recorded (state %s, reason %s); nothing recorded, repeat this step "
            "in a new session" % (check, state, reason))
    return record(check, ok, note, **data)


if __name__ == "__main__":
    # python p0lib.py reset: after a check that was cut short, put the engaged core back to the normal state
    if sys.argv[1:] == ["reset"]:
        require_state("ready")
        status, now = set_mode("vpn")
        write_rules(*base_rules())
        print("RESET mode %s, base rule files written" % now)
    else:
        print("usage: p0lib.py reset")
```

- [ ] **Step 5: Записать три шаблона конфигов**

Шаблон режима настройки, `C:\code\router\spike\plan0\tpl\setup.json`. Адаптера нет, `route.auto_detect_interface` не задан (раздел 7 спецификации):

```json
{
  "log": { "level": "info", "timestamp": true, "output": "C:/code/router/spike/plan0/out/setup.log" },
  "dns": {
    "servers": [{ "type": "local", "tag": "dns-local" }]
  },
  "endpoints": [
    {
      "type": "tailscale",
      "tag": "vpn",
      "state_directory": "__TS_STATE_DIR__",
      "hostname": "__TS_HOSTNAME__"
    }
  ],
  "outbounds": [{ "type": "direct", "tag": "direct" }],
  "route": { "default_domain_resolver": "dns-local" },
  "services": [
    { "type": "api", "tag": "api", "listen": "127.0.0.1", "listen_port": 19091, "secret": "__API_SECRET__" }
  ]
}
```

Шаблон удержания, `C:\code\router\spike\plan0\tpl\hold.json`. Адаптер поднят, но перехватывает только адрес `198.18.0.1` и не трогает DNS: программы продолжают ходить через системный VPN. Блок адаптера повторяет `spike\run\phase-b-preflight.json`, проверенный утром:

```json
{
  "log": { "level": "debug", "timestamp": true, "output": "C:/code/router/spike/run/phase-b.log" },
  "dns": {
    "servers": [
      { "type": "local", "tag": "dns-local" },
      { "type": "https", "tag": "dns-vpn", "server": "1.1.1.1", "detour": "vpn" }
    ],
    "rules": [
      { "clash_mode": "vpn-localdns", "action": "route", "server": "dns-local" },
      { "clash_mode": "direct-localdns", "action": "route", "server": "dns-local" },
      { "clash_mode": "alldirect", "action": "route", "server": "dns-local" }
    ],
    "final": "dns-vpn",
    "strategy": "ipv4_only",
    "reverse_mapping": true
  },
  "inbounds": [
    { "type": "mixed", "tag": "probe-vpn", "listen": "127.0.0.1", "listen_port": 17891 },
    { "type": "mixed", "tag": "probe-direct", "listen": "127.0.0.1", "listen_port": 17892 },
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "SplitLane",
      "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
      "auto_route": true,
      "strict_route": true,
      "route_address": ["198.18.0.1/32", "fdfe:dcba:9877::1/128"],
      "dns_mode": "disabled",
      "stack": "gvisor"
    }
  ],
  "endpoints": [
    {
      "type": "tailscale",
      "tag": "vpn",
      "state_directory": "__TS_STATE_DIR__",
      "hostname": "__TS_HOSTNAME__",
      "exit_node": "__EXIT_NODE__"
    }
  ],
  "outbounds": [{ "type": "direct", "tag": "direct", "domain_resolver": "dns-local" }],
  "route": {
    "rules": [
      { "ip_version": 6, "action": "reject" },
      { "inbound": "probe-vpn", "action": "route", "outbound": "vpn" },
      { "inbound": "probe-direct", "action": "route", "outbound": "direct" },
      { "port": 53, "action": "hijack-dns" },
      { "ip_cidr": ["172.19.0.0/30"], "action": "reject" },
      { "process_path_regex": ["(?i)^C:\\\\Program Files\\\\Tailscale\\\\(tailscaled|tailscale-ipn)\\.exe$"], "action": "route", "outbound": "direct" },
      { "process_path_regex": ["(?i)^C:\\\\CODE\\\\ROUTER\\\\SPIKE\\\\PLAN0\\\\BIN\\\\INLINE\\\\CURL-INLINE\\.EXE$"], "action": "route", "outbound": "direct" },
      { "ip_is_private": true, "action": "route", "outbound": "direct" },
      { "process_name": ["claude.exe", "bun.exe", "node.exe", "python.exe", "pythonw.exe"], "action": "route", "outbound": "vpn" },
      { "domain_suffix": ["anthropic.com", "claude.ai", "claude.com"], "ip_cidr": ["160.79.104.0/23"], "action": "route", "outbound": "vpn" },
      { "clash_mode": "alldirect", "action": "route", "outbound": "direct" },
      { "rule_set": "lane-direct", "action": "route", "outbound": "direct" },
      { "rule_set": "lane-vpn", "action": "route", "outbound": "vpn" },
      { "clash_mode": "direct", "action": "route", "outbound": "direct" },
      { "clash_mode": "direct-localdns", "action": "route", "outbound": "direct" },
      { "clash_mode": "vpn", "action": "route", "outbound": "vpn" },
      { "clash_mode": "vpn-localdns", "action": "route", "outbound": "vpn" }
    ],
    "rule_set": [
      { "type": "local", "tag": "lane-direct", "format": "source", "path": "C:/code/router/spike/plan0/rules/lane-direct.json" },
      { "type": "local", "tag": "lane-vpn", "format": "source", "path": "C:/code/router/spike/plan0/rules/lane-vpn.json" }
    ],
    "final": "vpn",
    "auto_detect_interface": true,
    "find_process": true,
    "default_domain_resolver": "dns-local"
  },
  "services": [
    { "type": "api", "tag": "api", "listen": "127.0.0.1", "listen_port": 19091, "secret": "__API_SECRET__" }
  ],
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:19090",
      "secret": "__CLASH_SECRET__",
      "default_mode": "vpn"
    }
  }
}
```

Основной шаблон, `C:\code\router\spike\plan0\tpl\main.json`. Форма повторяет проверенный `spike\run\phase-b-v2.json` (правила `sniff` нет) и порядок правил из 6.2. Номера правил в массиве `route.rules`: 0 — IPv6; 1, 2 — входы `probe-vpn` и `probe-direct`; 3 — порт 53; 4 — адреса адаптера; 5 — системный Tailscale; 6 — встроенное правило с `process_path_regex` для копии `curl-inline.exe` (проверка механизма правил 5 и 6 из 6.2); 7 — частные адреса; 8 — закрепление программ исполнителя за VPN по имени процесса (только для проверок); 9 — закрепление за VPN адресов и имён сервиса, через который работает исполнитель (только для проверок); 10 — режим `alldirect`; 11, 12 — файлы правил; 13–16 — режимы основной дорожки. Правило 8 стоит после правила 7 намеренно: обращения программ исполнителя к локальной сети идут напрямую, как в утренней проверке. Адаптер назван `SplitLane`, как в 6.1; утром он назывался иначе, на работу ядра имя не влияет.

Зачем правило 9. Утренние журналы ядра показывают, что ядро иногда не определяет владельца соединения (шесть строк `failed to search process: Access is denied` и одна `process not found`). Такое соединение проходит мимо правила 8 и в режимах `direct`, `direct-localdns`, `alldirect` ушло бы напрямую. Правило 9 удерживает на дорожке VPN всё, что идёт к диапазону `160.79.104.0/23` или к именам `anthropic.com`, `claude.ai`, `claude.com`, чьей бы программой это ни было; это же оставляет на VPN вкладки браузера пользователя с этими сайтами. В правиле `domain_suffix` и `ip_cidr` объединены по «или». В конфиге `p0-main-vpn-norevmap.json` (обратное сопоставление выключено) работает только часть с `ip_cidr`. То же правило стоит в шаблоне удержания.

```json
{
  "log": { "level": "debug", "timestamp": true, "output": "C:/code/router/spike/run/phase-b.log" },
  "dns": {
    "servers": [
      { "type": "local", "tag": "dns-local" },
      { "type": "https", "tag": "dns-vpn", "server": "1.1.1.1", "detour": "vpn" }
    ],
    "rules": [
      { "clash_mode": "vpn-localdns", "action": "route", "server": "dns-local" },
      { "clash_mode": "direct-localdns", "action": "route", "server": "dns-local" },
      { "clash_mode": "alldirect", "action": "route", "server": "dns-local" }
    ],
    "final": "dns-vpn",
    "strategy": "ipv4_only",
    "reverse_mapping": true
  },
  "inbounds": [
    { "type": "mixed", "tag": "probe-vpn", "listen": "127.0.0.1", "listen_port": 17891 },
    { "type": "mixed", "tag": "probe-direct", "listen": "127.0.0.1", "listen_port": 17892 },
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "SplitLane",
      "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
      "auto_route": true,
      "strict_route": true,
      "route_exclude_address": ["100.64.0.0/10", "fd7a:115c:a1e0::/48"],
      "stack": "gvisor"
    }
  ],
  "endpoints": [
    {
      "type": "tailscale",
      "tag": "vpn",
      "state_directory": "__TS_STATE_DIR__",
      "hostname": "__TS_HOSTNAME__",
      "exit_node": "__EXIT_NODE__"
    }
  ],
  "outbounds": [{ "type": "direct", "tag": "direct", "domain_resolver": "dns-local" }],
  "route": {
    "rules": [
      { "ip_version": 6, "action": "reject" },
      { "inbound": "probe-vpn", "action": "route", "outbound": "vpn" },
      { "inbound": "probe-direct", "action": "route", "outbound": "direct" },
      { "port": 53, "action": "hijack-dns" },
      { "ip_cidr": ["172.19.0.0/30"], "action": "reject" },
      { "process_path_regex": ["(?i)^C:\\\\Program Files\\\\Tailscale\\\\(tailscaled|tailscale-ipn)\\.exe$"], "action": "route", "outbound": "direct" },
      { "process_path_regex": ["(?i)^C:\\\\CODE\\\\ROUTER\\\\SPIKE\\\\PLAN0\\\\BIN\\\\INLINE\\\\CURL-INLINE\\.EXE$"], "action": "route", "outbound": "direct" },
      { "ip_is_private": true, "action": "route", "outbound": "direct" },
      { "process_name": ["claude.exe", "bun.exe", "node.exe", "python.exe", "pythonw.exe"], "action": "route", "outbound": "vpn" },
      { "domain_suffix": ["anthropic.com", "claude.ai", "claude.com"], "ip_cidr": ["160.79.104.0/23"], "action": "route", "outbound": "vpn" },
      { "clash_mode": "alldirect", "action": "route", "outbound": "direct" },
      { "rule_set": "lane-direct", "action": "route", "outbound": "direct" },
      { "rule_set": "lane-vpn", "action": "route", "outbound": "vpn" },
      { "clash_mode": "direct", "action": "route", "outbound": "direct" },
      { "clash_mode": "direct-localdns", "action": "route", "outbound": "direct" },
      { "clash_mode": "vpn", "action": "route", "outbound": "vpn" },
      { "clash_mode": "vpn-localdns", "action": "route", "outbound": "vpn" }
    ],
    "rule_set": [
      { "type": "local", "tag": "lane-direct", "format": "source", "path": "C:/code/router/spike/plan0/rules/lane-direct.json" },
      { "type": "local", "tag": "lane-vpn", "format": "source", "path": "C:/code/router/spike/plan0/rules/lane-vpn.json" }
    ],
    "final": "vpn",
    "auto_detect_interface": true,
    "find_process": true,
    "default_domain_resolver": "dns-local"
  },
  "services": [
    { "type": "api", "tag": "api", "listen": "127.0.0.1", "listen_port": 19091, "secret": "__API_SECRET__" }
  ],
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:19090",
      "secret": "__CLASH_SECRET__",
      "default_mode": "__DEFAULT_MODE__"
    }
  }
}
```

- [ ] **Step 6: Записать `patch_watchdog.py`**

Файл `C:\code\router\spike\plan0\patch_watchdog.py`, целиком. Это точная правка `watchdog.py`: тринадцать замен, каждая исходная вставка должна встретиться ровно один раз. Что добавляется:

- режим `hold`: как `preflight`, но ядро остаётся поднятым до команды, файла `stop.flag`, истечения `deadline.txt` или 15 минут; `tailscale set` в этом режиме не вызывается; команда `engage` останавливает это ядро и продолжает обычным, уже проверенным включением; конфиг, адаптер которого не ограничен полем `route_address` (такой адаптер перехватил бы трафик всех программ при включённом системном exit node), в этом режиме отклоняется до запуска ядра;
- запрос через `probe-direct` сразу после доказательства туннеля и до выключения системного exit node — это проверка (г) в точности на шаге 5 включения; результат только пишется в журнал; если ответа нет, запрос повторяется, всего не больше трёх раз с паузой в секунду (один тайм-аут не должен решать исход проверки); тайм-аут запроса 4 секунды; после запросов watchdog проверяет, что ядро живо, и, если запросы заняли больше 5 секунд, доказывает туннель заново (до 15 секунд); системный exit node выключается только после этого;
- аргумент `--max-fails` для задачи 5.

Правило watchdog сохраняется: ядро, через которое идёт трафик программ, не останавливается и watchdog не завершается, пока системный exit node не подтверждён включённым. Ядро режима `hold` трафик программ не несёт, поэтому его остановка всегда безопасна.

```python
"""SplitLane Plan 0: the exact change to spike/scripts/watchdog.py.

Adds: mode "hold" (preflight that stays up; an "engage" command continues as a
normal engage), a probe of the direct lane before the system exit node is
switched off, and --max-fails for the supervised health check.

Keeps the safety rules of the watchdog: the hold phase never calls
`tailscale set`; the engage path is the reviewed one; the core that carries
application traffic is never stopped while the system exit node is not
confirmed on.

Usage: python patch_watchdog.py <source watchdog.py> <destination>
Every old fragment must occur exactly once, otherwise nothing is written.
"""
import py_compile
import sys

EDITS = [
    # 0: module docstring
    ('''  engage     Start the core with a TUN config, prove the embedded node works while
''',
     '''  hold       Like preflight, but the core stays up (status "holding") so that more
             probes can run while the system exit node is still on. Never touches
             the system Tailscale settings. The command
             {"action": "engage", "config": "<file in run/>", "id": "..."} stops that
             core and continues as engage with the named config.
  engage     Start the core with a TUN config, prove the embedded node works while
'''),
    # 1: second probe port
    ('''PROBE_PORT = 17891
''',
     '''PROBE_PORT = 17891
PROBE_DIRECT_PORT = 17892
'''),
    # 2: hold time cap
    ('''HEALTH_EVERY = 10
''',
     '''HEALTH_EVERY = 10
HOLD_CAP_SECONDS = 15 * 60
'''),
    # 3: opener for the direct probe
    ('''PROBE = urllib.request.build_opener(urllib.request.ProxyHandler({
    "http": "http://127.0.0.1:%d" % PROBE_PORT,
    "https": "http://127.0.0.1:%d" % PROBE_PORT,
}))
''',
     '''PROBE = urllib.request.build_opener(urllib.request.ProxyHandler({
    "http": "http://127.0.0.1:%d" % PROBE_PORT,
    "https": "http://127.0.0.1:%d" % PROBE_PORT,
}))
PROBE_DIRECT = urllib.request.build_opener(urllib.request.ProxyHandler({
    "http": "http://127.0.0.1:%d" % PROBE_DIRECT_PORT,
    "https": "http://127.0.0.1:%d" % PROBE_DIRECT_PORT,
}))
'''),
    # 4: new state fields
    ('''    "last_deadline": None,
}
''',
     '''    "last_deadline": None,
    "max_fails": 3,
    "phase": "",
}
'''),
    # 5: direct probe
    ('''    return st2 is not None, {"ip": ip, "anthropic_http": st2, "leak": False}
''',
     '''    return st2 is not None, {"ip": ip, "anthropic_http": st2, "leak": False}


def direct_probe():
    """Through the core's second loopback proxy, which is routed to the direct outbound."""
    st, body = http_get(PROBE_DIRECT, "https://api.ipify.org", timeout=4)
    if st != 200:
        return {"ip": None, "error": body or st}
    ip = body.strip()
    return {"ip": ip, "differs_from_vpn": ip != EXPECT_IP}
'''),
    # 6: measure the direct lane while the system exit node is still on
    ('''        return False, proc, "%s (system exit node was not switched off)" % detail

    state["exit_node_cleared"] = True
''',
     '''        return False, proc, "%s (system exit node was not switched off)" % detail

    # Plan 0, check (g): the direct lane while the system exit node is still on.
    probe_started = time.time()
    for _ in range(3):
        measured = direct_probe()
        log("direct probe before switch: %s" % (measured,))
        if measured.get("ip"):
            break
        time.sleep(1)
    # The probes took time: the exit node is switched off only for a core that is still alive
    # and, if more than 5 s passed, only after the tunnel is proven once more.
    if proc.poll() is not None:
        return False, proc, "core exited rc=%s (system exit node was not switched off)" % proc.returncode
    if time.time() - probe_started > 5:
        ok, detail = wait_tunnel(proc, 15)
        if not ok:
            return False, proc, "%s (system exit node was not switched off)" % detail

    state["exit_node_cleared"] = True
'''),
    # 7: hold mode
    ('''def supervise(config, proc, started_at):
''',
     '''def hold(config):
    """Preflight that stays up. Never touches the system Tailscale settings.

    The adapter of a hold config captures no application traffic, so stopping
    this core is always safe; a config whose adapter is not limited by
    route_address is refused. Returns the name of a checked config when an
    "engage" command asks to go on; otherwise None, and the final status is
    already written.
    """
    ok, why = check_config(config)
    if not ok:
        refuse("config check failed: %s" % why)
        return None
    try:
        with open(os.path.join(RUN, config), "r", encoding="utf-8-sig") as f:
            inbounds = json.load(f).get("inbounds") or []
    except (OSError, ValueError):
        inbounds = [{"type": "tun"}]
    if any(i.get("type") == "tun" and not i.get("route_address") for i in inbounds):
        refuse("hold needs an adapter limited by route_address; this config would capture application traffic")
        return None
    proc = start_core(config)
    final = "hold_failed"
    info = {"reason": "unknown"}
    next_config = None
    try:
        err = wait_core_start(proc)
        if err:
            info = {"reason": err}
            return None
        ok, detail = wait_tunnel(proc, PREFLIGHT_PROBE_WAIT)
        if not ok:
            info = {"reason": str(detail)}
            return None
        set_deadline(READY_GRACE_SECONDS)
        t0 = time.time()
        write_status("holding", config=config, pid=proc.pid, detail=detail, exit_node_still_on=exit_node_on())
        final = "hold_done"
        cmd_errors = 0
        while True:
            time.sleep(1)
            now = time.time()
            if os.path.exists(STOP):
                info = {"reason": "stop requested"}
                return None
            dl = read_deadline()
            if dl is None or now > dl:
                info = {"reason": "heartbeat deadline expired"}
                return None
            if now - t0 > HOLD_CAP_SECONDS:
                info = {"reason": "hold time cap reached"}
                return None
            if proc.poll() is not None:
                final = "hold_failed"
                info = {"reason": "core exited rc=%s" % proc.returncode}
                return None
            if not os.path.exists(CMD):
                continue
            try:
                with open(CMD, "r", encoding="utf-8-sig") as f:
                    cmd = json.load(f)
            except (OSError, ValueError) as e:
                cmd_errors += 1
                if cmd_errors < 3:
                    continue  # probably half-written; try again on the next tick
                cmd = {"error": repr(e)}
            cmd_errors = 0
            try:
                os.remove(CMD)
            except OSError:
                continue  # do not risk running the same command twice
            state["cmd_id"] = cmd.get("id")
            action = cmd.get("action")
            log("command (hold): %s" % (cmd,))
            if action == "stop":
                info = {"reason": "stop command"}
                return None
            if action != "engage":
                write_status("holding", config=config, pid=proc.pid, cmd_error="unknown or unreadable command")
                continue
            new = os.path.basename(str(cmd.get("config", "")))
            if not os.path.isfile(os.path.join(RUN, new)):
                write_status("holding", config=config, pid=proc.pid, cmd_error="config not found")
                continue
            ok, why = check_config(new)
            if not ok:
                write_status("holding", config=config, pid=proc.pid, cmd_error="check failed: %s" % why)
                continue
            next_config = new
            return next_config
    finally:
        stop_core(proc)
        if next_config is None:
            write_status(final, exit_node_still_on=exit_node_on(), **info)
            state["final_status_written"] = True
        else:
            write_status("hold_to_engage", config=next_config)


def supervise(config, proc, started_at):
'''),
    # 8: configurable number of failed health checks
    ('''                if fails >= 3:
                    return rollback(proc, "health failed 3 times: %s" % (detail,))
''',
     '''                if fails >= state["max_fails"]:
                    return rollback(proc, "health failed %d times: %s" % (fails, detail))
'''),
    # 9: arguments
    ('''    ap.add_argument("--mode", choices=["preflight", "engage"], required=True)
''',
     '''    ap.add_argument("--mode", choices=["preflight", "hold", "engage"], required=True)
    ap.add_argument("--max-fails", type=int, default=3)
'''),
    # 10: state from arguments
    ('''    state["run_id"] = args.run_id
''',
     '''    state["run_id"] = args.run_id
    state["max_fails"] = max(3, min(args.max_fails, 12))
    state["phase"] = args.mode
'''),
    # 11: hold branch; after it the unchanged engage path runs
    ('''        if args.mode == "preflight":
            return preflight(args.config)

        if exit_node_on() is not True:
''',
     '''        if args.mode == "preflight":
            return preflight(args.config)

        if args.mode == "hold":
            next_config = hold(args.config)
            if next_config is None:
                return
            args.config = next_config
            state["phase"] = "engage"

        if exit_node_on() is not True:
'''),
    # 12: an exception in the hold phase is handled like one in preflight
    ('''        if args.mode == "preflight":
            stop_core(state["proc"])
            write_status("preflight_failed", reason="exception: %r" % (e,))
            state["final_status_written"] = True
''',
     '''        if state["phase"] in ("preflight", "hold"):
            stop_core(state["proc"])
            write_status("%s_failed" % state["phase"], reason="exception: %r" % (e,))
            state["final_status_written"] = True
'''),
]


def main():
    if len(sys.argv) != 3:
        print("usage: patch_watchdog.py <source> <destination>")
        return 2
    source, destination = sys.argv[1], sys.argv[2]
    with open(source, "r", encoding="utf-8", newline="") as f:
        text = f.read()
    if "def hold(config):" in text:
        print("ALREADY-PATCHED")
        return 0
    if "\r\n" in text:
        print("PATCH-FAILED: unexpected CRLF line endings")
        return 1
    for index, (old, new) in enumerate(EDITS):
        count = text.count(old)
        if count != 1:
            print("PATCH-FAILED: fragment %d occurs %d times" % (index, count))
            return 1
        text = text.replace(old, new)
    with open(destination, "w", encoding="utf-8", newline="") as f:
        f.write(text)
    py_compile.compile(destination, doraise=True)
    print("PATCHED %d fragments" % len(EDITS))
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 7: Применить правку к `watchdog.py`**

```powershell
if (-not (Test-Path C:\code\router\spike\scripts\watchdog.py.orig)) { Copy-Item C:\code\router\spike\scripts\watchdog.py C:\code\router\spike\scripts\watchdog.py.orig }
& C:\Python314\python.exe C:\code\router\spike\plan0\patch_watchdog.py C:\code\router\spike\scripts\watchdog.py C:\code\router\spike\scripts\watchdog.py
```

Ожидается: `PATCHED 13 fragments`. При повторном запуске: `ALREADY-PATCHED`. При `PATCH-FAILED` файл не изменён; остановиться и сравнить `watchdog.py` с `watchdog.py.orig`.

- [ ] **Step 8: Записать и выполнить самопроверку**

Самопроверка не запускает ядро, не вызывает `tailscale set` и не требует UAC. Она проверяет код, от которого зависит безопасность следующих задач.

Файл `C:\code\router\spike\plan0\selftest\fake_watchdog.py`, целиком:

```python
"""Plan 0 self-test: a fake of the watchdog protocol (status.json / cmd.json / stop.flag).

Takes the same arguments as the real watchdog, touches only the folder named in P0_FAKE_RUN,
starts no core and calls no Tailscale command.
"""
import argparse
import json
import os
import time

ap = argparse.ArgumentParser()
ap.add_argument("--mode", required=True)
ap.add_argument("--config", required=True)
ap.add_argument("--run-id", default="")
ap.add_argument("--not-after", type=float, default=0)
ap.add_argument("--max-fails", type=int, default=3)
args = ap.parse_args()
RUN = os.environ["P0_FAKE_RUN"]
STATUS = os.path.join(RUN, "status.json")
CMD = os.path.join(RUN, "cmd.json")
STOP = os.path.join(RUN, "stop.flag")
seq = 0
cmd_id = None


def status(state, **kw):
    global seq
    seq += 1
    data = {"run": args.run_id, "seq": seq, "state": state, "cmd_id": cmd_id, "time": int(time.time())}
    data.update(kw)
    tmp = STATUS + ".tmp"
    for _ in range(25):
        try:
            with open(tmp, "w", encoding="ascii") as f:
                json.dump(data, f)
            os.replace(tmp, STATUS)
            return
        except OSError:
            time.sleep(0.2)


if args.config == "silent.json":
    raise SystemExit(0)  # a watchdog that dies before its first status
for p in (STOP, CMD):
    if os.path.exists(p):
        os.remove(p)
status("starting", mode=args.mode, config=args.config)
time.sleep(0.5)
config = args.config
holding = args.mode == "hold"
if holding:
    status("holding", config=config, pid=1, detail={"ip": "203.0.113.10"}, exit_node_still_on=True)
elif args.config == "stall.json":
    status("engaging", config=config)
else:
    status("ready", config=config, pid=1, detail="{\"ip\": \"203.0.113.10\"}")
end = time.time() + 60
while time.time() < end:
    time.sleep(0.2)
    if os.path.exists(STOP):
        status("rolling_back", reason="stop requested")
        time.sleep(0.3)
        status("rolled_back", reason="stop requested", system_vpn_ok=True, ip="203.0.113.10")
        break
    if not os.path.exists(CMD):
        continue
    cmd = json.load(open(CMD, encoding="utf-8-sig"))
    os.remove(CMD)
    cmd_id = cmd.get("id")
    if cmd["action"] == "stop":
        status("rolling_back", reason="stop command")
        time.sleep(0.3)
        status("rolled_back", reason="stop command", system_vpn_ok=True, ip="203.0.113.10")
        break
    if cmd["config"] == "bad.json":
        status("holding" if holding else "ready", config=config, pid=1, cmd_error="check failed: nope")
        continue
    status("hold_to_engage" if cmd["action"] == "engage" else "restarting", config=cmd["config"])
    time.sleep(0.5)
    config = cmd["config"]
    holding = False
    status("ready", config=config, pid=2, detail="x")
```

Файл `C:\code\router\spike\plan0\selftest\selftest.py`, целиком. Он загружает исправленный `watchdog.py`, подменяет в нём все функции, которые трогают систему, и проходит двенадцать сценариев; затем проверяет чистые функции `p0lib.py`:

```python
"""Plan 0 self-test: the patched watchdog with every system-touching function replaced by a fake,
and the pure helpers of p0lib. Starts no core, calls no Tailscale command, needs no elevation.

usage: selftest.py [path to the patched watchdog]   default: spike/scripts/watchdog.py
"""
import contextlib
import importlib.util
import io
import itertools
import json
import os
import re
import shutil
import sys
import threading
import time

sys.path.insert(0, r"C:\code\router\spike\plan0")
import p0lib as L  # noqa: E402

TARGET = sys.argv[1] if len(sys.argv) > 1 else L.WATCHDOG
WORK = os.path.join(L.P0, "selftest", "work-py")
real_sleep = time.sleep
_time = time


class FastTime:
    shift = 0.0   # seconds a scenario adds to the watchdog's clock to play a slow step
    strftime = staticmethod(_time.strftime)

    @staticmethod
    def time():
        return _time.time() + FastTime.shift

    @staticmethod
    def sleep(seconds):
        real_sleep(min(seconds, 0.02))


class FakeProc:
    def __init__(self):
        self.pid = 4321
        self.returncode = None

    def poll(self):
        return self.returncode


def scenario(name, argv, script, tunnel_ok=True, health_ok=True, boom=False, probe_fails=0, probe_seconds=0,
             probe_ends_core=False):
    shutil.rmtree(WORK, ignore_errors=True)
    os.makedirs(WORK)
    spec = importlib.util.spec_from_file_location("wd", TARGET)
    wd = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(wd)
    wd.RUN = WORK
    for attr, fname in (("STATUS", "status.json"), ("DEADLINE", "deadline.txt"), ("STOP", "stop.flag"),
                        ("CMD", "cmd.json"), ("LOG", "watchdog.log"), ("CORE_LOG", "core.log"),
                        ("CORE_OUT", "core-out.log")):
        setattr(wd, attr, os.path.join(WORK, fname))
    FastTime.shift = 0.0
    wd.time = FastTime
    calls = {"ts": [], "start": [], "stop": 0, "exit_node": True, "health_ok": health_ok, "tunnel": []}
    # the shape of the real configs: the hold adapter is limited by route_address, the main one is not
    for config, text in (("hold.json", '{"inbounds": [{"type": "tun", "route_address": ["198.18.0.1/32"]}]}'),
                         ("main.json", '{"inbounds": [{"type": "tun"}]}')):
        with open(os.path.join(WORK, config), "w") as f:
            f.write(text)

    def start_core(config):
        proc = FakeProc()
        wd.state["proc"] = proc
        wd.state["stopping"] = False
        calls["start"].append(config)
        return proc

    def stop_core(proc, hard=False):
        wd.state["stopping"] = True
        if proc is not None and proc.returncode is None:
            proc.returncode = 0
            calls["stop"] += 1

    def ts_exit_node(on):
        calls["ts"].append(on)
        calls["exit_node"] = on
        return True

    def read_deadline_boom():
        raise RuntimeError("boom")

    def direct_probe():
        FastTime.shift += probe_seconds          # a probe that takes this long on the watchdog's clock
        if probe_ends_core:
            wd.state["proc"].returncode = 1      # the core exits while the probe runs
        if calls["probe_fails"] > 0:
            calls["probe_fails"] -= 1
            return {"ip": None, "error": "timed out"}
        return {"ip": "direct", "differs_from_vpn": True}

    def wait_tunnel(proc, limit):
        calls["tunnel"].append(limit)
        return (True, {"ip": "vpn"}) if tunnel_ok else (False, "no tunnel")

    calls["probe_fails"] = probe_fails
    wd.check_config = lambda config: (True, "")
    wd.start_core = start_core
    wd.stop_core = stop_core
    wd.wait_core_start = lambda proc: None
    wd.wait_tunnel = wait_tunnel
    wd.exit_node_on = lambda: calls["exit_node"]
    wd.ts_exit_node = ts_exit_node
    wd.other_core_running = lambda: False
    wd.health = lambda: (calls["health_ok"], {"ip": "vpn", "leak": False})
    wd.egress_ip = lambda opener=None: wd.EXPECT_IP
    wd.run = lambda cmd, timeout=30: (0, "", "")
    wd.direct_probe = direct_probe
    wd.HEALTH_EVERY = 0.05
    if boom:
        wd.read_deadline = read_deadline_boom
    seen = []

    def status():
        try:
            with open(wd.STATUS) as f:
                return json.load(f)
        except (OSError, ValueError):
            return {}

    def wait_state(*states, timeout=8):
        end = time.time() + timeout
        while time.time() < end:
            s = status()
            if s.get("state") and (not seen or seen[-1] != s["state"]):
                seen.append(s["state"])
            if s.get("state") in states:
                return s
            real_sleep(0.005)
        raise AssertionError("%s: timeout waiting for %s, seen %s" % (name, states, seen))

    def wait_field(key, value, timeout=5):
        end = time.time() + timeout
        while time.time() < end:
            if status().get(key) == value:
                return status()
            real_sleep(0.01)
        raise AssertionError("%s: %s never became %r: %s" % (name, key, value, status()))

    def command(**cmd):
        tmp = wd.CMD + ".tmp"
        with open(tmp, "w") as f:
            json.dump(cmd, f)
        os.replace(tmp, wd.CMD)

    old_argv = sys.argv
    sys.argv = ["watchdog.py"] + argv
    thread = threading.Thread(target=wd.main, daemon=True)
    thread.start()
    try:
        script(wd, calls, wait_state, wait_field, command)
    finally:
        sys.argv = old_argv
    thread.join(10)
    assert not thread.is_alive(), "%s: watchdog did not exit" % name
    print("ok   watchdog: %-24s states=%s tailscale-set calls=%s" % (name, seen, calls["ts"]))


def hold_engage_stop(wd, calls, wait_state, wait_field, command):
    wait_state("holding")
    assert calls["ts"] == [], "hold must not call tailscale set"
    command(action="engage", config="main.json", id="c1")
    s = wait_state("ready")
    assert s["cmd_id"] == "c1" and s["config"] == "main.json", s
    assert calls["ts"] == [False] and calls["start"] == ["hold.json", "main.json"] and calls["stop"] == 1
    with open(wd.LOG) as f:
        assert f.read().count("direct probe before switch") == 1   # answered at once: one measurement
    command(action="stop", hard=True, id="c2")
    s = wait_state("rolled_back")
    assert s["cmd_id"] == "c2" and s["system_vpn_ok"] is True
    assert calls["ts"] == [False, True] and calls["stop"] == 2


def probe_retried(wd, calls, wait_state, wait_field, command):
    wait_state("ready")
    with open(wd.LOG) as f:
        lines = [line for line in f if "direct probe before switch" in line]
    assert len(lines) == 3 and "'ip': None" in lines[0] and "'differs_from_vpn': True" in lines[2], lines
    assert calls["tunnel"] == [wd.PROBE_WAIT], calls["tunnel"]   # quick probes: the first proof stands
    command(action="stop", hard=True, id="c1")
    wait_state("rolled_back")
    assert calls["ts"] == [False, True]


def probe_core_lost(wd, calls, wait_state, wait_field, command):
    s = wait_state("rolled_back")
    assert "core exited rc=1 (system exit node was not switched off)" in s["reason"], s
    assert calls["ts"] == [True], calls["ts"]   # never switched off; the rollback only confirms that it is on


def probe_slow(wd, calls, wait_state, wait_field, command):
    wait_state("ready")
    assert calls["tunnel"] == [wd.PROBE_WAIT, 15], calls["tunnel"]   # proven once more before the switch
    command(action="stop", hard=True, id="c1")
    wait_state("rolled_back")
    assert calls["ts"] == [False, True]


def hold_full_adapter(wd, calls, wait_state, wait_field, command):
    s = wait_state("failed_no_change")
    assert "route_address" in s["reason"], s
    assert calls["start"] == [] and calls["ts"] == []


def hold_stop_flag(wd, calls, wait_state, wait_field, command):
    wait_state("holding")
    with open(wd.STOP, "w") as f:
        f.write("x")
    s = wait_state("hold_done")
    assert s["reason"] == "stop requested" and s["exit_node_still_on"] is True
    assert calls["ts"] == [] and calls["stop"] == 1


def hold_bad_commands(wd, calls, wait_state, wait_field, command):
    wait_state("holding")
    command(action="dance", id="c1")
    assert wait_field("cmd_id", "c1")["cmd_error"] == "unknown or unreadable command"
    command(action="engage", config="nope.json", id="c2")
    s = wait_field("cmd_id", "c2")
    assert s["cmd_error"] == "config not found" and s["state"] == "holding"
    command(action="stop", id="c3")
    s = wait_state("hold_done")
    assert s["reason"] == "stop command" and s["cmd_id"] == "c3" and calls["ts"] == []


def hold_deadline(wd, calls, wait_state, wait_field, command):
    wait_state("holding")
    tmp = wd.DEADLINE + ".tmp"
    with open(tmp, "w") as f:
        f.write(str(int(time.time()) - 5))
    os.replace(tmp, wd.DEADLINE)
    s = wait_state("hold_done")
    assert s["reason"] == "heartbeat deadline expired" and calls["ts"] == []


def hold_no_tunnel(wd, calls, wait_state, wait_field, command):
    s = wait_state("hold_failed")
    assert s["reason"] == "no tunnel" and calls["ts"] == [] and calls["stop"] == 1


def hold_exception(wd, calls, wait_state, wait_field, command):
    wait_state("hold_failed")
    end = time.time() + 5
    while time.time() < end:
        with open(wd.STATUS) as f:
            if "boom" in f.read():
                break
        real_sleep(0.01)
    else:
        raise AssertionError("exception not reported")
    assert calls["ts"] == [] and calls["stop"] == 1


def max_fails(expected):
    def script(wd, calls, wait_state, wait_field, command):
        wait_state("ready")
        calls["health_ok"] = False
        s = wait_state("rolled_back")
        assert s["reason"].startswith("health failed %d times" % expected), s
        assert calls["ts"] == [False, True]
    return script


def watchdog_tests():
    with open(TARGET, "r", encoding="utf-8") as f:
        if "def hold(config):" not in f.read():
            print("SELFTEST-FAILED: %s is not patched (run patch_watchdog.py first)" % TARGET)
            sys.exit(1)
    hold = ["--mode", "hold", "--config", "hold.json", "--run-id"]
    engage = ["--mode", "engage", "--config", "main.json", "--run-id"]
    scenario("hold, engage, stop", hold + ["r1"], hold_engage_stop)
    scenario("hold, stop.flag", hold + ["r2"], hold_stop_flag)
    scenario("hold, bad commands", hold + ["r3"], hold_bad_commands)
    scenario("hold, deadline expired", hold + ["r4"], hold_deadline)
    scenario("hold, no tunnel", hold + ["r5"], hold_no_tunnel, tunnel_ok=False)
    scenario("hold, exception", hold + ["r6"], hold_exception, boom=True)
    scenario("engage, --max-fails 5", engage + ["r7", "--max-fails", "5"], max_fails(5))
    scenario("engage, default fails", engage + ["r8"], max_fails(3))
    scenario("engage, probe retried", engage + ["r9"], probe_retried, probe_fails=2)
    scenario("engage, core lost", engage + ["r10"], probe_core_lost, probe_ends_core=True)
    scenario("engage, slow probe", engage + ["r11"], probe_slow, probe_seconds=6)
    scenario("hold, full adapter", ["--mode", "hold", "--config", "main.json", "--run-id", "r12"], hold_full_adapter)
    shutil.rmtree(WORK, ignore_errors=True)


def helper_tests():
    for name, path in L.COPIES.items():
        assert re.search(L.rx_file(path, upper=True), path), name
        assert re.search(L.rx_folder(os.path.dirname(path), upper=True), path), name
        assert not re.search(L.rx_file(path, upper=True), path + ".bak"), name
    assert re.search(L.rx_folder(os.path.dirname(L.COPIES["outer"])), L.COPIES["inner"])
    assert not re.search(L.rx_folder(os.path.dirname(L.COPIES["outer"])), L.COPIES["space"])
    assert L.quote_meta(r"C:\a (x86) [v1]\b.exe") == r"C:\\a \(x86\) \[v1\]\\b\.exe"
    assert L.rule_except("A", ["B"]) == {"type": "logical", "mode": "and", "rules": [
        {"process_path_regex": ["A"]}, {"process_path_regex": ["B"], "invert": True}]}
    print("ok   p0lib: regex builders and rule documents")
    info = L.parse_ts_status("vpn\n  State:     NeedsLogin\n  Log in:    https://login.example/a/abc\n")
    assert info == {"State": "NeedsLogin", "Log in": "https://login.example/a/abc"}, info
    rows = L.parse_exit_nodes("a.example\t100.64.0.1\tyes\tselected\nb.example\t100.64.0.2\tno\t-\n")
    assert len(rows) == 2 and rows[0]["status"] == "selected" and rows[1]["online"] == "no"
    assert L.parse_path("pong from a (100.64.0.1) via DERP(x) in 30ms\npong from a (100.64.0.1) via 192.0.2.1:41641 in 9ms\n") == "direct"
    assert L.parse_path("pong from a (100.64.0.1) via DERP(x) in 30ms\ndirect connection not established\n") == "relay"
    assert L.parse_path("pong from a (100.64.0.1) via peer relay x in 30ms\n") == "peer-relay"
    assert L.parse_path("") == "none"
    print("ok   p0lib: parsers of the `sing-box api` output")
    log = ("+0300 2026-11-13 10:36:07 INFO [649726994 0ms] router: found process path: C:\\x\\curl-a.exe\n"
           "+0300 2026-11-13 10:36:07 DEBUG [649726994 0ms] router: match[5] rule_set=lane-direct => route(direct)\n"
           "+0300 2026-11-13 10:36:08 INFO [11 0ms] router: found process path: C:\\x\\curl-b.exe\n"
           "+0300 2026-11-13 10:36:08 INFO [12 0ms] router: found process path: C:\\x\\curl-a.exe\n"
           "+0300 2026-11-13 10:36:08 DEBUG [12 0ms] router: pre-match[0] ip_version=6 => reject\n") + " " * 300
    assert L.matches_for_process(log, r"\\curl-a\.exe$") == [
        ("649726994", 5, "rule_set=lane-direct", "route(direct)"), ("12", 0, "ip_version=6", "reject")]
    assert L.matches_for_process(log, r"\\curl-b\.exe$") == [("11", None, "", "")]
    print("ok   p0lib: core log parser")
    circuit = L.euler_circuit(L.MODES)
    pairs = list(zip(circuit, circuit[1:]))
    assert circuit[0] == circuit[-1] == "vpn" and sorted(pairs) == sorted(itertools.permutations(L.MODES, 2))
    print("ok   p0lib: mode walk covers all %d ordered pairs" % len(pairs))
    assert L.lane_name(None, "a", "b") == "NONE" and L.lane_name("a", "a", "b") == "VPN"
    assert L.lane_name("b", "a", "b") == "DIRECT" and L.lane_name("c", "a", "b") == "OTHER"
    assert L.is_ipv4("192.0.2.1") and not L.is_ipv4("x") and not L.is_ipv4(None)
    # one answer from the wrong lane is tolerated (the core sometimes misses the owner), two are not
    real = L.egress_exe
    try:
        answers = iter(["b", "a", "a", "b", "b", "a", "a"])
        L.egress_exe = lambda exe, timeout=10: next(answers)
        assert L.lane_of("x", "VPN", "a", "b") == (True, ["DIRECT", "VPN", "VPN"])
        assert L.lane_of("x", "VPN", "a", "b") == (False, ["DIRECT", "DIRECT", "VPN"])
        assert L.lane_of("x", "VPN", "a", "b") == (True, ["VPN"])
        # a request without an answer is repeated and is not counted as an answer
        answers = iter([None, "a", None, "b", None, "a", "a"])
        assert L.lane_of("x", "VPN", "a", "b") == (True, ["NONE", "VPN"])
        assert L.lane_of("x", "VPN", "a", "b") == (True, ["NONE", "DIRECT", "NONE", "VPN", "VPN"])
        # too few answers: the script stops instead of recording a verdict
        answers = iter([None] * 6)
        try:
            with contextlib.redirect_stdout(io.StringIO()):
                L.lane_of("x", "VPN", "a", "b")
            raise AssertionError("six requests without an answer must stop the script")
        except SystemExit:
            pass
    finally:
        L.egress_exe = real
    print("ok   p0lib: two-of-three rule for lane answers")


if __name__ == "__main__":
    watchdog_tests()
    helper_tests()
    print("SELFTEST-OK")
```

Файл `C:\code\router\spike\plan0\selftest\selftest.ps1`, целиком. Он проверяет `lib.ps1` на подставном watchdog; запуск через UAC заменён обычным запуском, остальной код `lib.ps1` работает без изменений:

```powershell
# Plan 0 self-test of lib.ps1 against selftest\fake_watchdog.py.
# No elevation, no core, no Tailscale change; files are written only under plan0\selftest\work.
. C:\code\router\spike\plan0\lib.ps1
$work = 'C:\code\router\spike\plan0\selftest\work'
Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $work | Out-Null
$P0Run = $work
$P0Out = $work
$P0Watchdog = 'C:\code\router\spike\plan0\selftest\fake_watchdog.py'
$env:P0_FAKE_RUN = $work
foreach ($n in 'hold.json', 'main.json', 'main2.json', 'bad.json', 'stall.json', 'silent.json') {
    [IO.File]::WriteAllText((Join-Path $work $n), '{}')
}

# The real launcher code runs unchanged; only the elevation is taken out of Start-Process.
$P0TestCancel = $false
function Start-Process {
    param($FilePath, $ArgumentList, $Verb, $WindowStyle, $ErrorAction)
    if ($script:P0TestCancel) { throw 'The operation was canceled by the user.' }
    Microsoft.PowerShell.Management\Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -WindowStyle Hidden
}

$failed = 0
function Expect {
    param([string]$Name, [scriptblock]$Action, [string]$Pattern)
    $text = (& $Action 6>&1 | Out-String)
    if ($text -match $Pattern) { Write-Host "ok   $Name" }
    else { Write-Host "FAIL $Name - wanted /$Pattern/, got: $text"; $script:failed++ }
}

$libText = [IO.File]::ReadAllText('C:\code\router\spike\plan0\lib.ps1')
Expect 'lib.ps1 is ASCII' { if ($libText -match '[^\x00-\x7F]') { 'non-ascii' } else { 'ascii' } } '^ascii'
Write-P0Atomic -Path (Join-Path $work 'a.txt') -Text 'one'
Write-P0Atomic -Path (Join-Path $work 'a.txt') -Text 'two'
Expect 'atomic write replaces, no BOM' { ([IO.File]::ReadAllBytes((Join-Path $work 'a.txt')) -join ' ') } '^116 119 111'
Set-P0Heartbeat
$lead = [int64]([IO.File]::ReadAllText((Join-Path $work 'deadline.txt'))) - [int64](& $P0Python -c "import time; print(int(time.time()))")
Expect 'heartbeat is unix time + 400' { "$lead" } '^(39[89]|40[01])\s*$'
$P0TestCancel = $true
Expect 'cancelled UAC starts nothing' {
    Start-P0Watchdog -Mode hold -Config hold.json
    "session-file=$(Test-Path (Join-Path $work 'session.json')) status-file=$(Test-Path (Join-Path $work 'status.json'))"
} 'UAC-CANCELLED[\s\S]*session-file=False status-file=False'
$P0TestCancel = $false
Remove-Item -LiteralPath (Join-Path $work 'deadline.txt') -Force
Expect 'hold launch' {
    Start-P0Watchdog -Mode hold -Config hold.json
    "heartbeat-after-holding=$(Test-Path (Join-Path $work 'deadline.txt'))"
} 'P0-RESULT: state=holding[\s\S]*heartbeat-after-holding=True'
Expect 'rejected command returns at once' { Send-P0Command -Action engage -Config bad.json } 'P0-RESULT: state=holding .*cmd_error=check failed'
Expect 'engage from hold' { Send-P0Command -Action engage -Config main.json } 'P0-RESULT: state=ready .*config=main.json'
Expect 'restart' { Send-P0Command -Action restart -Config main2.json } 'P0-RESULT: state=ready .*config=main2.json'
Expect 'missing config refused' { Send-P0Command -Action restart -Config nope.json } 'P0-RESULT: REFUSED - config not found'
Expect 'second launch refused' { Start-P0Watchdog -Mode engage -Config main.json } 'P0-RESULT: REFUSED - the previous watchdog'
Expect 'session line' { Get-P0Session } 'P0-SESSION: state=ready .*engaged_minutes=0'
Expect 'stop' { Send-P0Command -Action stop } 'P0-RESULT: state=rolled_back .*system_vpn_ok=True'
Expect 'no command after the end' { Send-P0Command -Action restart -Config main.json } 'P0-RESULT: NO-LIVE-SESSION'
# A status left behind by a reboot: not terminal, and no python.exe is old enough to have written it.
[IO.File]::WriteAllText((Join-Path $work 'status.json'), '{"run":"dead0001","seq":7,"state":"ready","cmd_id":null,"time":1000000000,"config":"main.json"}')
[IO.File]::WriteAllText((Join-Path $work 'session.json'), '{"run":"dead0001","mode":"engage","started":1000000000,"engaged":1000000000}')
Expect 'stale status: launch refused' { Start-P0Watchdog -Mode engage -Config main.json } 'P0-RESULT: REFUSED - the previous watchdog'
Expect 'stale status: command not sent' {
    Send-P0Command -Action stop
    "stop-flag=$(Test-Path (Join-Path $work 'stop.flag')) cmd-file=$(Test-Path (Join-Path $work 'cmd.json'))"
} 'NO-LIVE-SESSION - stale status[\s\S]*stop-flag=True cmd-file=False'
Expect 'stale status: session line' { Get-P0Session } 'P0-SESSION: state=ready .*STALE'
Expect 'stale status is closed' { Close-P0StaleSession } 'P0-RESULT: state=restore_unverified .*reason=stale status closed'
Expect 'nothing left to close' { Close-P0StaleSession } 'P0-RESULT: NOTHING-TO-CLOSE'
Remove-Item -LiteralPath (Join-Path $work 'stop.flag') -Force -ErrorAction SilentlyContinue
$P0FirstStatusSeconds = 2
$P0AfterStopSeconds = 2
Expect 'silent watchdog is reported' { Start-P0Watchdog -Mode engage -Config silent.json } 'NO-STATUS[\s\S]*P0-RESULT: no status'
$P0AfterStopSeconds = 150
Expect 'addresses are masked' { Format-P0Status ([pscustomobject]@{ state = 'x'; reason = 'left through 203.0.113.10' }) } 'left through <ip>'
Microsoft.PowerShell.Management\Start-Process -FilePath $P0Python -ArgumentList @($P0Watchdog, '--mode', 'engage', '--config', 'stall.json', '--run-id', 'stall001') -WindowStyle Hidden
Start-Sleep -Seconds 2
Expect 'live watchdog is not closed' { Close-P0StaleSession } 'P0-RESULT: REFUSED - a python.exe older than the last status'
Expect 'poll timeout writes stop.flag' { Format-P0Status (Wait-P0Status -RunId 'stall001' -Target @('ready') -TimeoutSeconds 3) } 'POLL-TIMEOUT[\s\S]*state=rolled_back'
Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
if ($failed -eq 0) { Write-Host 'LIB-SELFTEST-OK' } else { Write-Host "LIB-SELFTEST-FAILED ($failed)" }
```

Запуск:

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\selftest\selftest.py
```

Ожидается (списки состояний в строках могут отличаться: состояние, сменившееся быстрее опроса, в список не попадает, а промежуточные `starting` и `rolling_back` могут в него попасть; важны слово `ok`, последнее состояние и список `tailscale-set calls`):

```text
ok   watchdog: hold, engage, stop       states=['holding', 'engaging', 'ready', 'rolled_back'] tailscale-set calls=[False, True]
ok   watchdog: hold, stop.flag          states=['holding', 'hold_done'] tailscale-set calls=[]
ok   watchdog: hold, bad commands       states=['holding', 'hold_done'] tailscale-set calls=[]
ok   watchdog: hold, deadline expired   states=['holding', 'hold_done'] tailscale-set calls=[]
ok   watchdog: hold, no tunnel          states=['hold_failed'] tailscale-set calls=[]
ok   watchdog: hold, exception          states=['holding', 'hold_failed'] tailscale-set calls=[]
ok   watchdog: engage, --max-fails 5    states=['engaging', 'ready', 'rolled_back'] tailscale-set calls=[False, True]
ok   watchdog: engage, default fails    states=['engaging', 'ready', 'rolled_back'] tailscale-set calls=[False, True]
ok   watchdog: engage, probe retried    states=['engaging', 'ready', 'rolled_back'] tailscale-set calls=[False, True]
ok   watchdog: engage, core lost        states=['engaging', 'rolled_back'] tailscale-set calls=[True]
ok   watchdog: engage, slow probe       states=['engaging', 'ready', 'rolled_back'] tailscale-set calls=[False, True]
ok   watchdog: hold, full adapter       states=['failed_no_change'] tailscale-set calls=[]
ok   p0lib: regex builders and rule documents
ok   p0lib: parsers of the `sing-box api` output
ok   p0lib: core log parser
ok   p0lib: mode walk covers all 20 ordered pairs
ok   p0lib: two-of-three rule for lane answers
SELFTEST-OK
```

Главное в первых шести строках: в режиме `hold` список `tailscale-set calls` пуст во всех сценариях, кроме перехода к включению. Девятая строка подтверждает повтор запроса через `probe-direct`: два запроса без ответа, третий с ответом, в журнале три строки `direct probe before switch`. Десятая: ядро завершилось, пока шёл запрос через `probe-direct`, — системный exit node не выключался (в списке нет `False`; единственный вызов `True` — подтверждение при откате). Одиннадцатая: запрос занял больше 5 секунд — перед выключением системного exit node туннель доказан второй раз. Двенадцатая: конфиг с полным адаптером в режиме `hold` отклонён до запуска ядра.

```powershell
& C:\code\router\spike\plan0\selftest\selftest.ps1
```

Ожидается:

```text
ok   lib.ps1 is ASCII
ok   atomic write replaces, no BOM
ok   heartbeat is unix time + 400
ok   cancelled UAC starts nothing
ok   hold launch
ok   rejected command returns at once
ok   engage from hold
ok   restart
ok   missing config refused
ok   second launch refused
ok   session line
ok   stop
ok   no command after the end
ok   stale status: launch refused
ok   stale status: command not sent
ok   stale status: session line
ok   stale status is closed
ok   nothing left to close
ok   silent watchdog is reported
ok   addresses are masked
ok   live watchdog is not closed
ok   poll timeout writes stop.flag
LIB-SELFTEST-OK
```

Строки `stale status` проверяют случай перезагрузки ПК при живом сеансе: в `status.json` осталось состояние `ready`, а процесса watchdog нет. Новый запуск по-прежнему отклоняется, команда не отправляется (при `stop` остаётся `stop.flag`), а `Close-P0StaleSession` закрывает состояние. Строка `live watchdog is not closed` подтверждает обратное: пока процесс, который мог записать состояние, жив, закрыть его нельзя.

Любая строка `FAIL` или отсутствие итоговой строки — остановиться и исправить файл, который проверяла эта строка. К задаче 3 нельзя переходить без `SELFTEST-OK` и `LIB-SELFTEST-OK`.

- [ ] **Step 9: Записать `prepare.py`**

Файл `C:\code\router\spike\plan0\prepare.py`, целиком:

```python
"""Plan 0, task 1: state checks, test copies, base rule files, every config rendered and checked.

Read-only towards the system: no core is started, Tailscale is only asked for its status.
"""
import subprocess

from p0lib import *


def state_checks():
    problems = 0

    def line(label, ok):
        nonlocal problems
        problems += 0 if ok else 1
        print("STATE %-34s %s" % (label, "OK" if ok else "WRONG"))

    version = subprocess.run([EXE, "version"], capture_output=True, timeout=30,
                             creationflags=NO_WINDOW).stdout.decode("utf-8", "replace")
    line("sing-box 1.14.4", "sing-box version 1.14.4" in version)
    with open(WATCHDOG, "r", encoding="utf-8") as f:
        line("watchdog patched (mode hold)", "def hold(config):" in f.read())
    line("no sing-box.exe running", not core_running())
    line("no live watchdog session", (read_json(STATUS) or {}).get("state") in TERMINAL)
    line("system exit node on", system_exit_node_on())
    prefs = subprocess.run([TS, "debug", "prefs"], capture_output=True, timeout=20,
                           creationflags=NO_WINDOW).stdout.decode("utf-8", "replace")
    line("system accept-dns off", '"CorpDNS": false' in prefs)
    line("embedded node state present", os.path.isfile(os.path.join(RUN, "ts-state", "tailscaled.state")))
    busy = []
    for port in (PROBE_VPN, PROBE_DIRECT, 19090, 19091):
        probe = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        try:
            probe.bind(("127.0.0.1", port))
        except OSError:
            busy.append(port)
        finally:
            probe.close()
    line("local ports of the checks free", not busy)
    line("this process leaves through VPN", egress_self() == local_values()["vpn_ip"])
    return problems


def main():
    os.makedirs(OUT, exist_ok=True)
    os.makedirs(RULES, exist_ok=True)
    problems = state_checks()
    ensure_copies()
    present = [name for name, path in COPIES.items() if os.path.isfile(path)]
    print("COPIES %d of %d present, node copies: %s" % (
        len(present), len(COPIES), os.path.isfile(NODE_VPN) and os.path.isfile(NODE_DIRECT)))
    problems += len(COPIES) - len(present)
    write_rules(*base_rules())
    jobs = [
        ("setup.json", SETUP_CONFIG, {}),
        ("setup.json", SETUP_FRESH_CONFIG, {"state_dir": FRESH_STATE, "hostname": "splitlane-plan0-fresh"}),
        ("hold.json", os.path.join(RUN, "p0-hold.json"), {}),
    ]
    jobs += [("main.json", os.path.join(RUN, "p0-main-%s.json" % mode), {"default_mode": mode}) for mode in MODES]
    jobs += [("main.json", os.path.join(RUN, "p0-main-vpn-norevmap.json"), {"reverse_mapping": False})]
    for template, out_path, extra in jobs:
        ok, message = render(template, out_path, **extra)
        print("%s %s %s" % ("CHECK-OK  " if ok else "CHECK-FAIL", os.path.basename(out_path), "" if ok else message))
        problems += 0 if ok else 1
        if template == "setup.json":
            continue
        # the two rules that keep the executor on the VPN lane (its programs by process name, its API by
        # destination for connections without a known owner) must precede every mode rule
        rules = (read_json(out_path) or {}).get("route", {}).get("rules", [])
        pin = next((i for i, r in enumerate(rules)
                    if r.get("process_name") == PIN_NAMES and r.get("outbound") == "vpn"), None)
        dest = next((i for i, r in enumerate(rules)
                     if DEST_PIN in (r.get("ip_cidr") or []) and r.get("outbound") == "vpn"), None)
        first_mode = next((i for i, r in enumerate(rules) if "clash_mode" in r), None)
        if None in (pin, dest, first_mode) or pin > first_mode or dest > first_mode:
            print("PIN-WRONG  %s" % os.path.basename(out_path))
            problems += 1
    print("PREPARE-OK" if problems == 0 else "PREPARE-FAILED (%d problems)" % problems)
    sys.exit(0 if problems == 0 else 1)


if __name__ == "__main__":
    main()
```

- [ ] **Step 10: Выполнить `prepare.py`**

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\prepare.py
```

Ожидается:

```text
STATE sing-box 1.14.4                    OK
STATE watchdog patched (mode hold)       OK
STATE no sing-box.exe running            OK
STATE no live watchdog session           OK
STATE system exit node on                OK
STATE system accept-dns off              OK
STATE embedded node state present        OK
STATE local ports of the checks free     OK
STATE this process leaves through VPN    OK
COPIES 9 of 9 present, node copies: True
CHECK-OK   p0-setup.json 
CHECK-OK   p0-setup-fresh.json 
CHECK-OK   p0-hold.json 
CHECK-OK   p0-main-vpn.json 
CHECK-OK   p0-main-vpn-localdns.json 
CHECK-OK   p0-main-direct.json 
CHECK-OK   p0-main-direct-localdns.json 
CHECK-OK   p0-main-alldirect.json 
CHECK-OK   p0-main-vpn-norevmap.json 
PREPARE-OK
```

Строка `WRONG`, `CHECK-FAIL` или `PIN-WRONG` — остановиться. `PIN-WRONG` означает, что в готовом конфиге правило 8 (программы исполнителя) или правило 9 (адреса сервиса исполнителя) отсутствует либо стоит после правил режимов: сверьте шаблон с текстом шага 5. `STATE system exit node on WRONG` означает, что системный VPN сейчас выключен: проверки рассчитаны на включённый; попросите пользователя выбрать exit node в приложении Tailscale и повторите шаг. `STATE no live watchdog session WRONG` сразу после перезагрузки ПК означает устаревший `status.json`: действуйте по строке `NO-LIVE-SESSION - stale status` таблицы состояний в Global Constraints.

- [ ] **Step 11: Убедиться, что в git ничего не появилось**

```powershell
git -C C:\code\router status --short
```

Ожидается: в выводе нет строк с `spike/`. Допустима только строка с файлом этого плана.

---

### Task 2: Режим настройки: (д) и (б) без адаптера

Ядро запускается без адаптера и без прав администратора, дважды: с состоянием уже вошедшего встроенного узла и с пустым состоянием. Система не меняется, системный exit node остаётся включённым, UAC не нужен. Оба ядра останавливает сам скрипт.

**Files:**
- Create: `C:\code\router\spike\plan0\check_setup.py`

**Interfaces:**
- Consumes: из задачи 1 — `p0lib.py` (`require_no_session`, `core_running`, `system_exit_node_on`, `system_exit_nodes`, `ts_status`, `ts_exit_nodes`, `ts_path`, `api_cli`, `whole_log`, `record`, `SETUP_CONFIG`, `SETUP_FRESH_CONFIG`, `SETUP_LOG`, `FRESH_STATE`), файлы `spike\plan0\out\p0-setup.json` и `p0-setup-fresh.json`.
- Produces: записи `B0-setup-api`, `D1-setup-state`, `B1-setup-secret`, `D2-setup-exit-nodes`, `D3-setup-data-path`, `D4-setup-log`, `D5-setup-login-link`, `B5-setup-api-link` в `spike\plan0\out\results.json`. Записи с буквой `D` отвечают на вопрос (д) — работает ли сам режим настройки; записи с буквой `B` — на вопрос (б), отдаёт ли данные служба API.

- [ ] **Step 1: Записать `check_setup.py`**

Файл `C:\code\router\spike\plan0\check_setup.py`, целиком:

```python
"""Plan 0, checks (d) and (b) in setup mode.

Starts a core WITHOUT an adapter, not elevated, twice: with the state of the
already logged-in embedded node, and with an empty state. The system is not
changed; the system exit node stays on. Each core is stopped before the script
returns.
"""
import subprocess

from p0lib import *


def start_core(config):
    if os.path.exists(SETUP_LOG):
        os.remove(SETUP_LOG)
    out = open(os.path.join(OUT, "setup-stdout.log"), "ab")
    proc = subprocess.Popen([EXE, "run", "-c", config, "-D", RUN], stdout=out, stderr=subprocess.STDOUT,
                            stdin=subprocess.DEVNULL, creationflags=NO_WINDOW)
    end = time.time() + 30
    while time.time() < end:
        if proc.poll() is not None:
            return proc, "core exited rc=%s" % proc.returncode
        if "sing-box started" in whole_log(SETUP_LOG):
            return proc, None
        time.sleep(0.5)
    return proc, "no start line within 30 s"


def stop_core(proc):
    if proc.poll() is None:
        proc.kill()
        proc.wait(timeout=10)


def existing_login():
    v = local_values()
    proc, err = start_core(SETUP_CONFIG)
    try:
        if err:
            record("D1-setup-state", False, err)
            return
        info = {}
        end = time.time() + 60
        while time.time() < end:
            info = ts_status()
            if info.get("State") in ("Running", "Starting"):
                break
            time.sleep(2)
        state = info.get("State")
        # (b): does the API service answer at all? (d): does the logged-in node get its state? Two questions.
        api_ok = info.get("rc") == 0 and "State" in info
        record("B0-setup-api", api_ok, "API service answers in setup mode: exit code %s" % info.get("rc"))
        if not api_ok:
            print("SKIP D1-setup-state, D2-setup-exit-nodes, B1-setup-secret: the API service did not answer")
        elif state == "NeedsLogin" or "Log in" in info:
            print("SKIP D1-setup-state, D2-setup-exit-nodes: the embedded node is not logged in (State=%s); "
                  "log in with Task 4, then repeat this task" % state)
        else:
            record("D1-setup-state", state in ("Running", "Starting"),
                   "State=%s, login link offered: %s" % (state, "Log in" in info), state=state)
        if api_ok:
            rc, _, _ = api_cli(["tailscale", "status"], secret="wrong-secret")
            record("B1-setup-secret", rc != 0, "a wrong secret is rejected: exit code %s" % rc)
        if api_ok and state in ("Running", "Starting"):
            rows, api, same = [], {}, False
            end = time.time() + 30
            while time.time() < end:
                _, rows = ts_exit_nodes()
                api = {r["ip"]: r["online"] == "yes" for r in rows}
                same = bool(rows) and api == system_exit_nodes()
                if same:
                    break
                time.sleep(2)
            mine = api.get(v["exit_node"]) is True
            record("D2-setup-exit-nodes", same and mine,
                   "exit nodes listed: %d, the system's exit node listed and online: %s, same set and same online "
                   "flags as the system client reports: %s" % (len(rows), mine, same), count=len(rows))
            path = ts_path(v["exit_node"])
            record("D3-setup-data-path", None, "ping of the exit node in setup mode: %s (none is expected)" % path,
                   path=path)
        log = whole_log(SETUP_LOG)
        record("D4-setup-log", None, "info-level log has the start line: %s, a login prompt: %s" % (
            "sing-box started" in log, "Waiting for authentication" in log))
    finally:
        stop_core(proc)


def fresh_login_link():
    shutil.rmtree(FRESH_STATE, ignore_errors=True)
    proc, err = start_core(SETUP_FRESH_CONFIG)
    try:
        if err:
            record("D5-setup-login-link", False, err)
            return
        pattern = r"Waiting for authentication: (https://\S+)"
        m = None
        end = time.time() + 45
        while time.time() < end and not m:
            m = re.search(pattern, whole_log(SETUP_LOG))
            time.sleep(1)
        # (d): the core gets a login link at all. (b): the API service hands out the same link.
        record("D5-setup-login-link", bool(m), "login link in the info-level log within 45 s: %s" % bool(m))
        if m:
            info = {}
            end = time.time() + 15
            while time.time() < end:
                info = ts_status()
                if info.get("Log in"):
                    break
                time.sleep(1)
            same = info.get("Log in") == m.group(1)
            record("B5-setup-api-link", same, "the API service returns the same link: %s, State=%s" % (
                same, info.get("State")), state=info.get("State"))
    finally:
        stop_core(proc)
        time.sleep(1)
        shutil.rmtree(FRESH_STATE, ignore_errors=True)


def main():
    require_no_session()
    if core_running():
        die("a sing-box.exe is already running")
    if not system_exit_node_on():
        die("the system exit node is not on; this check is about setup mode with it on")
    existing_login()
    fresh_login_link()
    print("SETUP-DONE core running afterwards: %s" % core_running())


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Выполнить проверку**

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\check_setup.py
```

Ожидается (число exit node — сколько их в сети; `State` может быть `Starting` или `Running`):

```text
PASS B0-setup-api API service answers in setup mode: exit code 0
PASS D1-setup-state State=Starting, login link offered: False
PASS B1-setup-secret a wrong secret is rejected: exit code 1
PASS D2-setup-exit-nodes exit nodes listed: 1, the system's exit node listed and online: True, same set and same online flags as the system client reports: True
INFO D3-setup-data-path ping of the exit node in setup mode: none (none is expected)
INFO D4-setup-log info-level log has the start line: True, a login prompt: False
PASS D5-setup-login-link login link in the info-level log within 45 s: True
PASS B5-setup-api-link the API service returns the same link: True, State=NeedsLogin
SETUP-DONE core running afterwards: False
```

Вторая часть проверки запускает ядро с пустой папкой состояния и именем устройства `splitlane-plan0-fresh`. Вход по полученной ссылке никто не выполняет, поэтому новое устройство в сети Tailscale не появляется; папка состояния удаляется.

- [ ] **Step 3: Оценить результат**

Критерии:

| Запись | Вопрос | Условие `PASS` | Что значит `FAIL` |
|---|---|---|---|
| `B0-setup-api` | (б) | за 60 секунд команда `sing-box api tailscale status` завершилась с кодом 0 и вернула поле `State` | служба API в режиме настройки не отвечает: запасной способ раздела 7 |
| `D1-setup-state` | (д) | узел с сохранённым входом за 60 секунд получил состояние `Starting` или `Running` | режим настройки при включённом системном exit node не работает: вопрос владельцу продукта |
| `B1-setup-secret` | (б) | запрос с неверным секретом завершился с ненулевым кодом | секрет службы API не защищает её; записывается в отчёт |
| `D2-setup-exit-nodes` | (б) и (д) | за 30 секунд служба API отдала список, который совпадает с тем, что системный клиент считает exit node (те же адреса), признаки «в сети» совпадают, exit node системного клиента помечен «в сети» | список или признаки неверны: запасной способ раздела 7 (список берётся у системного клиента) |
| `D5-setup-login-link` | (д) | ядро с пустым состоянием узла за 45 секунд записало ссылку для входа в журнал уровня `info` | режим настройки при включённом системном exit node не получает ссылку: вопрос владельцу продукта |
| `B5-setup-api-link` | (б) | служба API за 15 секунд отдала ту же ссылку, что в журнале | служба API ссылку не отдаёт: запасной способ раздела 7 (ссылка берётся из журнала) |

`D1` записывается, только если служба API ответила и узел не требует входа: без ответа службы состояние узнать нечем, а узел без входа — не свойство режима настройки. В этих случаях скрипт печатает строку `SKIP` с причиной. `B1` записывается, только если служба ответила; `D2` — только при состоянии `Starting` или `Running`; `B5` — только если ссылка есть в журнале.

Что это значит для плана 1:

- (д) подтверждена, если `D1`, `D2` и `D5` — `PASS`. Завершение входа по ссылке в режиме настройки проверяет задача 4 (запись `Z5-relogin`); если задача 4 не выполнялась, в приложении A эта строка останется «не проверялось».
- Для решения по разделу 7 нужны `B0`, `D2`, `B5` здесь и `B3-normal-api` в задаче 3. Любой `FAIL` среди них — запасной способ из раздела 7; он согласован в спецификации и ответа владельца продукта не ждёт.
- `FAIL` в `D1` или `D5` — другое дело: не работает сам режим настройки, и запасной способ раздела 7 (журнал ядра) его не заменит. `report.py` печатает такие записи в строке `OWNER-QUESTION`; туда же попадает `FAIL Z5-relogin` из задачи 4. Остальные задачи плана при этом выполняются.
- Строка `SKIP ... the embedded node is not logged in`: встроенный узел потерял вход. Выполните задачу 4 с шага 2 (скрипт сам увидит `NeedsLogin` и направит к шагу 5), затем повторите эту задачу.

Если последняя строка не `SETUP-DONE core running afterwards: False`, остановите оставшееся ядро (оно запущено без прав администратора):

```powershell
Stop-Process -Name sing-box -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 1; if (Get-Process -Name sing-box -ErrorAction SilentlyContinue) { 'STILL-RUNNING' } else { 'STOPPED' }
```

Ожидается: `STOPPED`.

---

### Task 3: Один сеанс с адаптером: (г), (б), (а), (в), (к)

Один запуск watchdog, один запрос UAC. Сначала удержание: адаптер поднят, системный exit node включён, программы не затронуты. Затем включение деления и проверки. В конце — откат.

Правила на всю задачу:

- Задачу целиком выполняет исполнитель, который сам ведёт разговор с пользователем, без перерывов; подчинённому агенту она не передаётся. Каждая команда начинается с `Set-P0Heartbeat`; пауза между командами — не больше 400 секунд.
- Все команды задачи запускаются инструментом PowerShell с тайм-аутом 600000 мс.
- Если инструмент вернул управление по тайм-ауту, либо скрипт закончился ошибкой Python (`Traceback`) или любым `ABORT`, кроме `ABORT SESSION-NOT-READY` и `ABORT SESSION-LOST`, ядро могло остаться в режиме с прямой основной дорожкой и с проверочными файлами правил. Сразу выполните команду ниже; ожидается `RESET mode vpn, base rule files written`. Если вместо этого напечатано `ABORT` или режим не `vpn`, выполните шаг 12.

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\p0lib.py reset
```

- Перед шагами 7–11 полезно узнать возраст сеанса командой ниже. Если `engaged_minutes` не меньше 35, закончите текущий шаг, выполните шаг 12 и начните новый сеанс командой `Start-P0Watchdog -Mode engage -Config p0-main-vpn.json` (нужен пользователь: UAC), затем продолжайте с прерванного шага.

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Get-P0Session
```

- Если любой скрипт напечатал `LEFT-IN-MODE` с режимом, отличным от `vpn`, сразу выполните шаг 12: ядро осталось в режиме с прямой основной дорожкой и не переключается.
- Если скрипт напечатал `ABORT SESSION-LOST`, сеанс откатился, пока скрипт работал, и записи этого шага не сделаны. Строка `LEFT-IN-MODE None` при этом ожидаема: ядра уже нет. Выполните шаг 12 (первая команда напечатает `NO-LIVE-SESSION`, проверка системы должна закончиться `PREPARE-OK`), прочитайте причину отката и повторите шаг целиком в новом сеансе (новый запрос UAC, с согласия пользователя). Откат сеанса никогда не является результатом проверки ядра.
- Если сеанс откатился сам между шагами (строка `P0-RESULT: state=rolled_back` или `ABORT SESSION-NOT-READY`), прочитайте причину командой `Get-P0Session`. Причина `traffic left through <ip> instead of the exit node` в режимах `direct`, `direct-localdns`, `alldirect` означает, что ядро не определило владельца запроса самого watchdog и запрос ушёл по основной дорожке; это наблюдение записывается в отчёт (спецификация, 5.3: один такой ответ утечкой не считается). Новый сеанс — новый запрос UAC, с согласия пользователя.
- `ABORT MEASUREMENT-FAILED` — сбой измерения, а не свойство ядра: запросы копии остались без ответа либо не прошёл контроль `V0` в шаге 7. Запись этого измерения не сделана; после команды `reset` шаг повторяется. Для пакетов UDP в шаге 8 повтор один, дальнейшие действия описаны там же.
- `FAIL`, в тексте которого из причин только `dns-no-answer` (нужный сервер DNS выбран, но не ответил за 5 секунд дважды), не относится к 6.3: шаг повторяется один раз. Если при повторе то же самое, запись остаётся: `report.py` не учитывает её в решении 1, печатает в строке `OWNER-QUESTION` как сбой пути DNS (6.4), и вопрос возвращается владельцу продукта; решение 1 принимается по остальным записям.

**Files:**
- Create: `C:\code\router\spike\plan0\check_hold.py`
- Create: `C:\code\router\spike\plan0\check_modes.py`
- Create: `C:\code\router\spike\plan0\check_rules.py`
- Create: `C:\code\router\spike\plan0\check_revmap.py`
- Create: `C:\code\router\spike\plan0\check_api.py`

**Interfaces:**
- Consumes: из задачи 1 — `lib.ps1` (`Set-P0Heartbeat`, `Start-P0Watchdog`, `Send-P0Command`, `Get-P0Session`), `p0lib.py` (в том числе `record_live`, `session_state`, `lane_of`, `confirm_pin`, `clash`, `connections`, `system_exit_nodes`), конфиги `spike\run\p0-hold.json`, `p0-main-<режим>.json`, `p0-main-vpn-norevmap.json`, копии программ, файлы правил.
- Produces: записи `G0-hold-system-vpn`, `G1-hold-probe-vpn`, `G2-hold-probe-direct`, `G3-engage-probe-direct`, `B2-hold-api`, `A0-mode-list`, `A1-start-<режим>` (пять), `A2-transitions-all-pairs`, `A2-transitions-from-<режим>` (четыре), `A3-unknown-mode`, `A4-mode-case`, `A5-open-connections`, `V0-control`, `V1-space-folder`, `V2-brackets-file`, `V3-cyrillic-file`, `V4-nested-invert`, `V5-vpn-file-direct-main`, `V6-hot-reload`, `V7-system-path`, `V8-broken-file`, `V9-inline-regex`, `K1-host-names`, `K5-host-alias` (если проверочное имя во время проверки остаётся псевдонимом), `K2-tcp-lanes`, `K3-udp-vpn-sizes`, `K4-udp-direct`, при провале `K3` — контрольные `K3n-udp-vpn-sizes`, `K4n-udp-direct` (`check_revmap.py udp`) и повторная запись `K3-udp-vpn-sizes`, `K4-udp-direct` (`check_revmap.py again`), `B3-normal-api`, `B4-normal-api-extras`; при отказе закрепления — `A-pin`; при ошибке чтения файла правил — `V1-reload-error`. Все записи задачи делает `record_live()`: после отката сеанса запись не появляется.

- [ ] **Step 1: Записать пять скриптов проверок**

Файл `C:\code\router\spike\plan0\check_hold.py`, целиком:

```python
"""Plan 0, check (g) and check (b) with the adapter up while the system exit node is still on.

usage: check_hold.py          in watchdog state "holding" (hold config, adapter captures nothing)
       check_hold.py engaged  in state "ready": reads what the watchdog measured right before the switch
"""
from p0lib import *


def holding():
    require_state("holding")
    v = local_values()
    on = system_exit_node_on()
    own = egress_self()
    record_live("G0-hold-system-vpn", on and own == v["vpn_ip"],
                "system exit node on: %s, this process still leaves through it: %s" % (on, own == v["vpn_ip"]),
                _session="holding")
    vpn = [egress_probe(PROBE_VPN) for _ in range(3)]
    record_live("G1-hold-probe-vpn", all(ip == v["vpn_ip"] for ip in vpn),
                "probe-vpn left through the exit node %d of 3 times" % sum(ip == v["vpn_ip"] for ip in vpn),
                _session="holding")
    direct = [egress_probe(PROBE_DIRECT) for _ in range(3)]
    good = [ip for ip in direct if ip and ip != v["vpn_ip"]]
    record_live("G2-hold-probe-direct", len(good) == 3 and len(set(good)) == 1,
                "probe-direct answered %d of 3 times, with an address other than the VPN one %d times, "
                "distinct addresses: %d" % (sum(ip is not None for ip in direct), len(good), len(set(good))),
                _session="holding")
    heartbeat()
    info = ts_status()
    _, rows = ts_exit_nodes()
    selected = any(r["ip"] == v["exit_node"] and r["status"] == "selected" for r in rows)
    path = ts_path(v["exit_node"])
    record_live("B2-hold-api", info.get("State") == "Running" and selected and path != "none",
                "State=%s, exit nodes listed: %d, configured exit node marked selected: %s, path to it: %s" % (
                    info.get("State"), len(rows), selected, path),
                _session="holding", state=info.get("State"), path=path)
    mode, modes = get_mode()
    record_live("A0-mode-list", mode == "vpn" and sorted(modes) == sorted(MODES),
                "GET /configs: mode=%s, mode-list is exactly the five modes: %s" % (
                    mode, sorted(modes) == sorted(MODES)),
                _session="holding", mode_list=modes)
    shutil.copyfile(CORE_LOG, os.path.join(OUT, "hold-core.log"))
    heartbeat()


def engaged():
    status = require_state("ready")
    log = whole_log(WATCHDOG_LOG)
    start = log.rfind("run=%s " % status["run"])
    lines = [line for line in log[max(start, 0):].splitlines() if "direct probe before switch:" in line]
    m = re.search(r"'ip': '([0-9.]+)'", lines[-1]) if lines else None
    same = False
    if m:
        # the provider's address is the one the direct lane shows now, with the system exit node off;
        # lanes() stops the script if it cannot be measured or equals the VPN address
        vpn, direct = lanes()
        same = m.group(1) == direct
    # the addresses are only compared, never printed
    record_live("G3-engage-probe-direct", same,
                "watchdog measured probe-direct before switching the system exit node off (full adapter): "
                "%d measurement(s), the last one got an answer: %s, from the address the direct lane has after "
                "the switch: %s" % (len(lines), bool(m), same))


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "engaged":
        engaged()
    else:
        holding()
```

Файл `C:\code\router\spike\plan0\check_modes.py`, целиком:

```python
"""Plan 0, check (a): clash_mode for routes and DNS, switched through PATCH /configs.

usage: check_modes.py start <mode>  the core must have STARTED in <mode> (default_mode), nothing switched yet;
                                    for a mode other than vpn it then goes from <mode> to every other mode
                                    and back (8 transitions)
       check_modes.py euler         all 20 ordered transitions between the five modes; start and end: vpn;
                                    then an unknown mode, the mode name in another case, and what a mode
                                    switch does to an open connection

Modes whose main lane is direct are entered only after confirm_pin(); every run ends in mode vpn, whatever
stopped it: everything after the session check runs inside one try/finally.
"""
import subprocess

from p0lib import *

HOLD_OPEN = ("const s = require('tls').connect(443, 'example.org', { servername: 'example.org' }, () => {});"
             "s.on('error', () => {}); setTimeout(() => process.exit(0), 20000);")


def hard_of(problems):
    return [p for p in problems if not p.startswith("note:")]


def only_dns_no_answer(problems):
    """True when every hard problem is an unanswered DNS query: a matter of the DNS path (spec 6.4), not of modes."""
    hard = hard_of(problems)
    return bool(hard) and all(p.startswith("dns-no-answer") for p in hard)


def owner_misses(problems):
    return sum(p.startswith("note: owner-miss") for p in problems)


def verify(mode, tail, vpn, direct, full):
    """Problems found while the core is in <mode>; an empty list means the mode behaves as designed."""
    problems = []
    checks = [("free", MAIN_LANE[mode])]
    if full:
        checks += [("pin-vpn", "DIRECT" if mode == "alldirect" else "VPN"), ("pin-direct", "DIRECT")]
    for name, want in checks:
        ok, seen = lane_of(COPIES[name], want, vpn, direct)
        if not ok:
            problems.append("%s copy: wanted %s, saw %s" % (name, want, seen))
        elif any(lane not in (want, "NONE") for lane in seen):
            problems.append("note: owner-miss, %s copy saw %s" % (name, seen))
        elif len(seen) > 1:
            problems.append("note: no-answer, %s copy saw %s" % (name, seen))
    answer = dns_probe(tail)
    if answer["seen"] and not answer["answered"]:
        answer = dns_probe(tail)  # one more try: an unanswered query is not yet a verdict
    if not answer["seen"]:
        problems.append("dns: query not seen in the core log")
    elif answer["server"] != DNS_SERVER[mode]:
        problems.append("dns: wanted %s, got %s" % (DNS_SERVER[mode], answer["server"]))
    elif not answer["answered"]:
        problems.append("dns-no-answer: server %s chosen, no answer in 5 s" % answer["server"])
    heartbeat()
    return problems


def walk(sequence, tail, vpn, direct):
    """Follow the mode sequence; returns (failed transitions, every problem and note found on the way)."""
    failures, found, verified, streak = [], [], set(), 0
    for source, target in zip(sequence, sequence[1:]):
        if session_state()[0] != "ready":
            die("SESSION-LOST during %s -> %s; nothing recorded" % (source, target))
        status, now = set_mode(target)
        problems = []
        if status != 204 or now != target:
            problems.append("PATCH answered HTTP %s, GET /configs says %s" % (status, now))
        else:
            problems = verify(target, tail, vpn, direct, full=target not in verified)
            verified.add(target)
        hard = hard_of(problems)
        found += problems
        print("  %-16s -> %-16s %s" % (source, target, "OK" if not hard else "; ".join(hard)))
        if hard:
            failures.append("%s -> %s: %s" % (source, target, "; ".join(hard)))
        # an unanswered DNS query does not stop the walk: the mode itself was entered and checked
        streak = streak + 1 if hard and not only_dns_no_answer(problems) else 0
        if streak >= 3:
            failures.append("stopped after three failed transitions in a row")
            break
    return failures, found


def held(node):
    """Start a node copy that keeps one TLS connection open; returns (process, connection id, outbound)."""
    proc = subprocess.Popen([node, "-e", HOLD_OPEN], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                            creationflags=NO_WINDOW)
    end = time.time() + 12
    while time.time() < end and proc.poll() is None:
        for c in connections():
            meta = c.get("metadata") or {}
            if meta.get("processPath", "").lower() == node.lower() and meta.get("destinationPort") == "443":
                return proc, c.get("id"), (c.get("chains") or ["?"])[0]
        time.sleep(0.2)
    return proc, None, None


def outbound_of(connection_id):
    for c in connections():
        if c.get("id") == connection_id:
            return (c.get("chains") or ["?"])[0]
    return None


def open_connections(vpn, direct):
    """Spec 6.3, fourth property: a mode switch leaves an open connection where it is; closing it by id works.

    Only the renamed copy node-vpn.exe opens the connection. Ends in mode vpn.
    """
    notes, good, measured, close_failed = [], True, True, False
    for before, after, old, new_lane in (("vpn", "alldirect", "vpn", "DIRECT"), ("alldirect", "vpn", "direct", "VPN")):
        set_mode(before)
        proc, cid, opened = held(NODE_VPN)
        kept = fresh = closed = None
        try:
            if cid:
                set_mode(after)
                time.sleep(1.5)
                kept = outbound_of(cid)
                fresh, _ = lane_of(COPIES["pin-vpn"], new_lane, vpn, direct)
                if kept:
                    clash("DELETE", "/connections/" + cid)
                    time.sleep(1)
                    closed = outbound_of(cid) is None
                    close_failed = close_failed or not closed
        finally:
            proc.kill()
            set_mode("vpn")
        if not cid:
            measured = False
            notes.append("%s->%s: not measured, the held connection did not appear in the connection list" % (
                before, after))
        else:
            notes.append("%s->%s: opened on %s, after the switch still on %s, new request follows the new mode: %s, "
                         "closed by id: %s" % (before, after, opened, kept, fresh, closed))
            good = good and opened == old and kept == old and bool(fresh) and bool(closed)
        heartbeat()
    record_live("A5-open-connections", good if measured or not good else None, "; ".join(notes),
                close_failed=close_failed)


def main():
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    want = sys.argv[2] if len(sys.argv) > 2 else ""
    if not (command == "euler" or (command == "start" and want in MODES)):
        die("usage: check_modes.py start <mode> | euler")
    require_state("ready")
    try:
        vpn, direct = lanes()
        ok, why = confirm_pin()
        print("PIN %s %s" % ("OK" if ok else "FAILED", why))
        if not ok:
            record_live("A-pin", False, why)
            die("the executor's programs are not provably pinned to the VPN lane; no direct-lane mode will be entered")
        tail = LogTail()
        mode, modes = get_mode()
        if command == "start":
            problems = [] if mode == want else ["GET /configs says %s right after start" % mode]
            if sorted(modes) != sorted(MODES):
                problems.append("mode-list is %s" % modes)
            problems += verify(want, tail, vpn, direct, full=True) if mode == want else []
            hard = hard_of(problems)
            record_live("A1-start-%s" % want, not hard,
                        "core started in %s: mode read back, unpinned and pinned copies and DNS as designed" % want
                        if not hard else "; ".join(hard), dns_only=only_dns_no_answer(problems))
            if mode == want and want != "vpn":
                sequence = [want]
                for other in MODES:
                    if other != want:
                        sequence += [other, want]
                failures, found = walk(sequence, tail, vpn, direct)
                record_live("A2-transitions-from-%s" % want, not failures,
                            "%d transitions from %s to every other mode and back, failures: %d, retries: %d" % (
                                len(sequence) - 1, want, len(failures), owner_misses(found))
                            if not failures else "; ".join(failures)[:600],
                            retries=owner_misses(found), dns_only=only_dns_no_answer(found))
        else:
            if mode != "vpn":
                die("euler starts from vpn, the core was in %s; switched back to vpn, run the step again" % mode)
            sequence = euler_circuit(MODES)
            failures, found = walk(sequence, tail, vpn, direct)
            record_live("A2-transitions-all-pairs", not failures,
                        "%d ordered transitions, failures: %d, retries: %d" % (
                            len(sequence) - 1, len(failures), owner_misses(found))
                        if not failures else "; ".join(failures)[:600],
                        retries=owner_misses(found), dns_only=only_dns_no_answer(found))
            before = get_mode()[0]
            status, now = set_mode("p0-unknown-mode")
            record_live("A3-unknown-mode", status == 204 and now == before,
                        "unknown mode: PATCH answered HTTP %s, mode before %s, after %s" % (status, before, now))
            status, now = set_mode("VPN-LOCALDNS")
            record_live("A4-mode-case", None,
                        "PATCH with VPN-LOCALDNS: HTTP %s, mode read back as %s" % (status, now))
            open_connections(vpn, direct)
    finally:
        status, now = set_mode("vpn")
        print("LEFT-IN-MODE %s%s" % (now, "" if now == "vpn" else " - NOT vpn: stop the session now"))
        heartbeat()


if __name__ == "__main__":
    main()
```

Файл `C:\code\router\spike\plan0\check_rules.py`, целиком. Выражения в файлах правил записаны заглавными буквами, а пути на диске — нет: так проверяется сравнение без учёта регистра, в том числе для кириллицы. Псевдопуть `:System` кладётся в файл дорожки VPN, а не прямой: доказательством служит строка журнала с именем сработавшего файла.

```python
"""Plan 0, check (v): process_path_regex in the hot-reloaded rule files.

Paths with spaces, brackets and Cyrillic, patterns written in upper case (the
paths on disk are not), nested rules through a logical rule with invert, the
VPN lane file while the main lane is direct, reload timing, the ":System"
pseudo-path, a broken file, a built-in (inline) regex rule.
Run in watchdog state "ready", core in mode vpn. Ends with the base rule files.

usage: check_rules.py             all checks
       check_rules.py no-direct   the same without the sub-step in mode direct (the user did not allow
                                  the direct main lane); V5 is then not recorded
"""
import subprocess

from p0lib import *

RELOAD_LIMIT = 3.0


def reload_error(tail):
    m = re.search(r"reload rule-set [^\n]*", tail.text())
    return m.group(0)[:200] if m else ""


def stage_rules(with_space=True, nested=False):
    direct, vpn = base_rules()
    if with_space:
        direct.append(rule(rx_folder(os.path.dirname(COPIES["space"]), upper=True)))
    direct.append(rule(rx_file(COPIES["brackets"], upper=True)))
    direct.append(rule(rx_file(COPIES["cyr"], upper=True)))
    if nested:
        direct.append(rule_except(rx_folder(os.path.dirname(COPIES["outer"]), upper=True),
                                  [rx_file(COPIES["inner"], upper=True)]))
    vpn.append(rule(rx_file(COPIES["inner"], upper=True)))
    vpn.append(rule("(?i)^:SYSTEM$"))
    return direct, vpn


def expect(check, cases, vpn, direct, extra=""):
    """cases: [(copy name, wanted lane)]. One record for the group."""
    bad = []
    for name, want in cases:
        ok, seen = lane_of(COPIES[name], want, vpn, direct)
        if not ok:
            bad.append("%s: wanted %s, saw %s" % (name, want, seen))
    heartbeat()
    return record_live(check, not bad, ("; ".join(bad) if bad else ", ".join("%s=%s" % c for c in cases)) + extra)


def main():
    no_direct = sys.argv[1:] == ["no-direct"]
    if sys.argv[1:] and not no_direct:
        die("usage: check_rules.py [no-direct]")
    require_state("ready")
    mode, _ = get_mode()
    if mode != "vpn":
        set_mode("vpn")
        die("this check starts in mode vpn, the core was in %s; switched back to vpn, run the step again" % mode)
    vpn, direct = lanes()
    tail = LogTail()
    try:
        # control: with the base rules none of these copies is matched, all take the main lane
        write_rules(*base_rules())
        time.sleep(2)
        if not expect("V0-control", [(n, "VPN") for n in ("space", "brackets", "cyr", "outer", "inner")], vpn, direct):
            die("MEASUREMENT-FAILED: the control failed - with the base rule files a test copy does not take "
                "the main lane; V1-V9 are not recorded, run the reset command, wait 5 s and repeat the step")

        # stage 1: plain rules; the first reload is timed
        write_rules(*stage_rules())
        took = wait_lane(COPIES["space"], "DIRECT", vpn, direct, limit=6.0)
        if took is None:
            # diagnostic: is it the rename that the core does not notice?
            for name in ("lane-direct.json", "lane-vpn.json"):
                path = os.path.join(RULES, name)
                with open(path, "r", encoding="utf-8") as f:
                    text = f.read()
                with open(path, "w", encoding="utf-8", newline="\n") as f:
                    f.write(text)
            again = wait_lane(COPIES["space"], "DIRECT", vpn, direct, limit=6.0)
            record_live("V6-hot-reload", False, "rename over the file was not applied within 6 s; "
                        "rewriting in place applied: %s; core log: %s" % (
                            again is not None, reload_error(tail) or "-"))
        expect("V1-space-folder", [("space", "DIRECT")], vpn, direct)
        expect("V2-brackets-file", [("brackets", "DIRECT")], vpn, direct)
        expect("V3-cyrillic-file", [("cyr", "DIRECT")], vpn, direct)
        expect("V9-inline-regex", [("inline", "DIRECT")], vpn, direct, " (rule in the config, not in a file)")
        error = reload_error(tail)
        if error:
            record_live("V1-reload-error", False, "core log: " + error)

        # stage 2: nested rules of different lanes through a logical rule with invert
        tail.mark()
        write_rules(*stage_rules(nested=True))
        wait_lane(COPIES["outer"], "DIRECT", vpn, direct, limit=6.0)
        error = reload_error(tail)
        nested_ok = expect("V4-nested-invert", [("outer", "DIRECT"), ("inner", "VPN")], vpn, direct,
                           "; core log: " + error if error else "")

        # the VPN lane file while the main lane is direct; a sub-step that is not run leaves no record
        ok, why = (False, "") if no_direct else confirm_pin()
        if no_direct:
            print("SKIP V5-vpn-file-direct-main not run: the user did not allow the direct main lane")
        elif not ok:
            record_live("A-pin", False, why)
            print("SKIP V5-vpn-file-direct-main not run, pin not confirmed")
        else:
            status, now = set_mode("direct")
            try:
                # a direct main lane is the precondition; giving it is the business of check (a), not of (v)
                produced = status == 204 and now == "direct" and lane_of(COPIES["free"], "DIRECT", vpn, direct)[0]
                if produced:
                    # the nested copy is judged only if the logical rule works (V4): without it the direct
                    # lane file catches that copy in any mode, and that is decision 3 (8.2), not (v)
                    cases = [("pin-vpn", "VPN")] + ([("inner", "VPN")] if nested_ok else [])
                    expect("V5-vpn-file-direct-main", cases, vpn, direct,
                           "; unpinned copy=DIRECT" + ("" if nested_ok else "; nested copy not judged: V4 failed"))
                else:
                    print("SKIP V5-vpn-file-direct-main not run: mode direct did not give a direct main lane "
                          "(HTTP %s, mode %s); that is check (a), not (v)" % (status, now))
            finally:
                set_mode("vpn")

        # reload timing in both directions
        if took is not None:
            write_rules(*stage_rules(with_space=False, nested=True))
            off = wait_lane(COPIES["space"], "VPN", vpn, direct, limit=6.0)
            write_rules(*stage_rules(nested=True))
            on = wait_lane(COPIES["space"], "DIRECT", vpn, direct, limit=6.0)
            times = [took, off, on]
            good = all(t is not None and t <= RELOAD_LIMIT for t in times)
            record_live("V6-hot-reload", good, "seconds from the write to the start of the first request that "
                        "follows the rewritten file (add, remove, add): %s; limit %.0f s" % (times, RELOAD_LIMIT),
                        seconds=times)
        heartbeat()

        # the ":System" pseudo-path: the kernel SMB client opens a TCP connection to a documentation address
        tail.mark()
        helper = subprocess.Popen([sys.executable, "-c", "import os; os.path.exists(r'\\\\192.0.2.1\\p0')"],
                                  creationflags=NO_WINDOW)
        seen = tail.wait(r"router: found process path: :System\s*$", timeout=20)
        helper.kill()
        if not seen:
            record_live("V7-system-path", None, "no connection owned by :System appeared within 20 s; not verified")
        else:
            time.sleep(1)
            hits = matches_for_process(tail.text() + " " * 300, r"^:System$")
            good = [h for h in hits if h[2].startswith("rule_set=lane-vpn") and h[3] == "route(vpn)"]
            record_live("V7-system-path", bool(good),
                        ":System connections seen: %d, matched by the VPN lane file: %d" % (len(hits), len(good)))
        heartbeat()

        # a broken file: the core must report it and keep the previous rules
        tail.mark()
        good_direct, good_vpn = stage_rules(nested=True)
        write_rule_file("lane-direct.json", good_direct + [rule("(?i)^C:\\\\broken(")])
        reported = tail.wait(r"reload rule-set lane-direct[^\n]*", timeout=6)
        ok, seen_lane = lane_of(COPIES["space"], "DIRECT", vpn, direct)
        write_rule_file("lane-direct.json", good_direct)
        record_live("V8-broken-file", bool(reported) and ok,
                    "error line in the core log: %s; previous rules stayed in force: %s; line: %s" % (
                        bool(reported), ok, reported.group(0)[:160] if reported else "-"))
    finally:
        status, now = set_mode("vpn")
        write_rules(*base_rules())
        heartbeat()
        print("RULES-RESTORED base rule files written")
        print("LEFT-IN-MODE %s%s" % (now, "" if now == "vpn" else " - NOT vpn: stop the session now"))


if __name__ == "__main__":
    main()
```

Файл `C:\code\router\spike\plan0\check_revmap.py`, целиком:

```python
"""Plan 0, check (k): dns.reverse_mapping gives host names in the connection list, the no-sniff path still works.

usage: check_revmap.py           K1 host names, K2 TCP by lanes, K3 large UDP through the VPN lane, K4 UDP direct,
                                 K5 what is shown for a name that is an alias (CNAME)
       check_revmap.py udp       only the UDP part, recorded as K3n/K4n; needs the config without reverse_mapping
       check_revmap.py again     only the UDP part once more with the main config, recorded as K3/K4 again:
                                 does the picture of the first run come back?
       check_revmap.py selftest  the K3 rules on scripted answers; needs no core and no session
Run in watchdog state "ready", core in mode vpn, base rule files.
"""
import subprocess

from p0lib import *

# The core maps an address to the owner name of the A record. For a zone apex that is the name the
# program asked for; for an alias it is the canonical name at the end of the CNAME chain.
HOST = "example.org"
ALIAS = "www.wikipedia.org"   # an alias (CNAME); verified at run time, K5 is skipped if it stops being one
HOLD_OPEN = ("const s = require('tls').connect(443, '%s', { servername: '%s' }, () => {});"
             "s.on('error', () => {}); setTimeout(() => process.exit(0), 15000);")
PADS = (0, 600, 1100, 1200, 1300, 1400)   # padding of the STUN request: datagrams of 20 to 1424 bytes
ATTEMPTS = 3                              # per datagram size
CONFIG = {"": "p0-main-vpn.json", "again": "p0-main-vpn.json", "udp": "p0-main-vpn-norevmap.json"}


def host_seen(node, outbound, host=HOST):
    """Open a TLS connection from a node copy, find it in the Clash API connection list."""
    subprocess.run(["ipconfig", "/flushdns"], capture_output=True, timeout=15, creationflags=NO_WINDOW)
    proc = subprocess.Popen([node, "-e", HOLD_OPEN % (host, host)], stdout=subprocess.DEVNULL,
                            stderr=subprocess.DEVNULL, creationflags=NO_WINDOW)
    try:
        end = time.time() + 12
        while time.time() < end and proc.poll() is None:
            for c in connections():
                meta = c.get("metadata") or {}
                if meta.get("processPath", "").lower() == node.lower() and meta.get("destinationPort") == "443":
                    return {"host": meta.get("host"), "outbound": (c.get("chains") or ["?"])[0] == outbound,
                            "rule": c.get("rule")}
            time.sleep(0.2)
        return None
    finally:
        proc.kill()


def payload(pad):
    """Size of the datagram stun.js sends for a padding: 20-byte header, 4-byte attribute header, the padding."""
    return 20 + (4 + pad - pad % 4 if pad else 0)


def stun(node, server, port, pads):
    """{payload size: address the server saw, or a word such as TIMEOUT}.

    spike/scripts/stun.js run by a node copy; it sends one datagram per padding and waits 4 s for each answer.
    """
    try:
        p = subprocess.run([node, STUN_JS, server, str(port), ",".join(str(pad) for pad in pads)],
                           capture_output=True, timeout=60, creationflags=NO_WINDOW)
    except subprocess.TimeoutExpired:
        return {}
    answers = {}
    for line in p.stdout.decode("utf-8", "replace").splitlines():
        m = re.match(r"udp payload\s+(\d+) B: (\S+)", line)
        if m:
            answers[int(m.group(1))] = m.group(2).split(":")[0]
    return answers


def stun_lanes(node, server, port, pads, vpn, direct):
    """{payload size: [lane of every attempt]}; the last element is the result for that size.

    A datagram without an answer (lane NONE) is sent again, ATTEMPTS times at most: one lost datagram
    is not a property of the core. An answer ends the attempts for its size, whatever lane it came from.
    """
    seen = {payload(pad): [] for pad in pads}
    left = list(pads)
    for _ in range(ATTEMPTS):
        answers = stun(node, server, port, left)
        heartbeat()
        for pad in left:
            ip = answers.get(payload(pad))
            seen[payload(pad)].append(lane_name(ip if is_ipv4(ip) else None, vpn, direct))
        left = [pad for pad in left if seen[payload(pad)][-1] == "NONE"]
        if not left:
            break
    return seen


def k3_verdict(seen):
    """(ok, lanes, late, lost) for the answers to the VPN lane copy; ok is None when no size was answered.

    PASS: every size was answered from the VPN lane, even if not at the first attempt. FAIL: an answer
    came from another lane, or some sizes stayed without an answer ATTEMPTS times in a row while others
    were answered (this is how the loss of large datagrams looked with protocol sniffing on, spec 6.1).
    No answer for any size is not a verdict: there is nothing to judge.
    """
    lanes_seen = {str(size): tries[-1] for size, tries in seen.items()}
    lost = [size for size, tries in seen.items() if tries[-1] == "NONE"]
    late = [size for size, tries in seen.items() if len(tries) > 1 and tries[-1] != "NONE"]
    ok = None if len(lost) == len(seen) else all(lane == "VPN" for lane in lanes_seen.values())
    return ok, lanes_seen, late, lost


def udp(vpn, direct, suffix, again=False):
    earlier = ((read_json(RESULTS) or {}).get("K3-udp-vpn-sizes") or {}).get("data", {})
    before = earlier.get("lanes")                   # the K3 record as it stands
    first = earlier.get("first_run") or before      # the picture of the first run survives a second `again`
    if again and not first:
        die("`check_revmap.py again` needs a K3-udp-vpn-sizes record to compare with; nothing recorded")
    ok, lanes_seen, late, lost = k3_verdict(stun_lanes(NODE_VPN, "stun.l.google.com", 19302, PADS, vpn, direct))
    if ok is not None:
        note = "STUN through the VPN lane, payload size -> lane: %s" % lanes_seen
        if late:
            note += "; answered only when sent again: %s" % late
        if lost:
            note += "; no answer in %d attempts: %s" % (ATTEMPTS, lost)
        if suffix:
            note += "; same picture as with reverse_mapping on: %s" % (before == lanes_seen)
        data = {"lanes": lanes_seen}
        if again:
            note += "; same picture as in the first run: %s" % (first == lanes_seen)
            data["first_run"] = first
        record_live("K3%s-udp-vpn-sizes" % suffix, ok, note, **data)
    seen = []
    for server, port in (("stun.l.google.com", 19302), ("stun.cloudflare.com", 3478)):
        seen += [tries[-1] for tries in stun_lanes(NODE_DIRECT, server, port, (0,), vpn, direct).values()]
    verdict = False if "VPN" in seen else True if "DIRECT" in seen else None
    record_live("K4%s-udp-direct" % suffix, verdict,
                "STUN from the direct lane copy, two servers: %s" % seen, lanes=seen)
    heartbeat()
    if ok is None:
        die("MEASUREMENT-FAILED: the STUN server answered none of the %d datagram sizes sent through the VPN "
            "lane, %d attempts each (the same server answered the direct lane copy: %s); K3%s-udp-vpn-sizes "
            "is not recorded, repeat the command" % (len(PADS), ATTEMPTS, seen[0] != "NONE", suffix))


def selftest():
    """The K3 rules on scripted answers instead of stun.js."""
    vpn, direct, t = "203.0.113.10", "192.0.2.1", "TIMEOUT"   # documentation addresses

    def play(script):
        """script: {padding: [answer to every attempt]}; None = stun.js printed no line for that padding."""
        runs = []

        def scripted(node, server, port, pads):
            runs.append(list(pads))
            answers = {pad: script.get(pad, [vpn]).pop(0) for pad in pads}
            return {payload(pad): text for pad, text in answers.items() if text is not None}

        globals().update(stun=scripted, heartbeat=lambda: None)
        return k3_verdict(stun_lanes("node", "server", 0, PADS, vpn, direct)), runs

    every = list(PADS)
    (ok, lanes_seen, late, lost), runs = play({})
    assert ok is True and not late and not lost and runs == [every], "every size answered at once"
    (ok, lanes_seen, late, lost), runs = play({0: [None, vpn], 1300: [t, vpn]})
    assert ok is True and late == [20, 1324] and not lost and runs == [every, [0, 1300]], "one loss is sent again"
    (ok, lanes_seen, late, lost), runs = play({1300: [t, t, t], 1400: [t, t, t]})
    assert ok is False and lost == [1324, 1424] and runs == [every, [1300, 1400], [1300, 1400]], "three losses"
    assert lanes_seen == {"20": "VPN", "624": "VPN", "1124": "VPN", "1224": "VPN", "1324": "NONE", "1424": "NONE"}
    (ok, lanes_seen, late, lost), runs = play({600: [direct]})
    assert ok is False and lanes_seen["624"] == "DIRECT" and not lost and runs == [every], "an answer is final"
    (ok, lanes_seen, late, lost), runs = play({pad: [t, t, t] for pad in PADS})
    assert ok is None and lost == [payload(pad) for pad in PADS] and len(runs) == ATTEMPTS, "silence: no verdict"
    print("ok   check_revmap: a lost datagram is sent again; FAIL needs a wrong lane or three losses in a row")


def main():
    arg = sys.argv[1] if len(sys.argv) > 1 else ""
    if arg == "selftest":
        selftest()
        return
    if arg not in CONFIG:
        die("usage: check_revmap.py [udp | again | selftest]")
    status = require_state("ready")
    running = os.path.basename(str(status.get("config")))
    if running != CONFIG[arg]:
        die("SESSION-NOT-READY: `%s` needs the core started with %s, status.json says %s; nothing recorded" % (
            ("check_revmap.py " + arg).strip(), CONFIG[arg], running))
    mode, _ = get_mode()
    if mode != "vpn":
        set_mode("vpn")
        die("this check starts in mode vpn, the core was in %s; switched back to vpn, run the step again" % mode)
    vpn, direct = lanes()
    if arg:
        udp(vpn, direct, "n" if arg == "udp" else "", again=arg == "again")
        return
    tail = LogTail()
    found = {"vpn": host_seen(NODE_VPN, "vpn"), "direct": host_seen(NODE_DIRECT, "direct")}
    if not all(found.values()):
        die("MEASUREMENT-FAILED: the connection of a node copy to %s did not appear in the connection list; "
            "nothing recorded, repeat the step" % HOST)
    good = all(f and f["host"] == HOST and f["outbound"] for f in found.values())
    logged = bool(re.search(r"found reserve mapped domain: %s" % re.escape(HOST), tail.text()))
    record_live("K1-host-names", good,
                "connection list shows the host name for the VPN lane copy: %s, for the direct "
                "lane copy: %s; each on its own outbound: %s; core log names the mapping: %s" % (
                    bool(found["vpn"]) and found["vpn"]["host"] == HOST,
                    bool(found["direct"]) and found["direct"]["host"] == HOST,
                    all(f and f["outbound"] for f in found.values()), logged),
                rules=[f["rule"] if f else None for f in found.values()])
    heartbeat()
    try:
        canonical = socket.gethostbyname_ex(ALIAS)[0].rstrip(".").lower()
    except OSError:
        canonical = ""
    if not canonical or canonical == ALIAS:
        print("SKIP K5-host-alias not recorded: %s does not resolve as an alias (canonical name: %s)" % (
            ALIAS, canonical or "none"))
    else:
        alias = host_seen(NODE_VPN, "vpn", ALIAS)
        if not alias:
            print("SKIP K5-host-alias not recorded: the connection to %s did not appear in the connection list" % ALIAS)
        else:
            shown = alias["host"]
            record_live("K5-host-alias", None, "for a connection to the alias %s the list shows: %s (%s)" % (
                ALIAS, shown or "nothing", "the name asked for" if shown == ALIAS else "not the name asked for"),
                same=shown == ALIAS)
    heartbeat()
    cases = [("pin-vpn", "VPN"), ("pin-direct", "DIRECT")]
    bad = []
    for name, want in cases:
        ok, seen = lane_of(COPIES[name], want, vpn, direct)
        if not ok:
            bad.append("%s: wanted %s, saw %s" % (name, want, seen))
    record_live("K2-tcp-lanes", not bad, "; ".join(bad) if bad else "pinned copies leave through their lanes")
    heartbeat()
    udp(vpn, direct, "")


if __name__ == "__main__":
    main()
```

Файл `C:\code\router\spike\plan0\check_api.py`, целиком:

```python
"""Plan 0, check (b) in normal mode: the API service of an engaged core.

B3 is what spec section 7 requires of the primary data source: node state, the exit node list with
the flags "can be an exit node" and "online". B4 holds the extras the spec can live without.
Run in watchdog state "ready".
"""
from p0lib import *


def main():
    require_state("ready")
    v = local_values()
    info = ts_status()
    rows, same = [], False
    for _ in range(3):
        _, rows = ts_exit_nodes()
        same = bool(rows) and {r["ip"]: r["online"] == "yes" for r in rows} == system_exit_nodes()
        if same:
            break
        time.sleep(2)
    mine = [r for r in rows if r["ip"] == v["exit_node"]]
    selected = bool(mine) and mine[0]["status"] == "selected"
    online = bool(mine) and mine[0]["online"] == "yes"
    rc_wrong, _, _ = api_cli(["tailscale", "status"], secret="wrong-secret")
    core = (info.get("State") == "Running" and "Log in" not in info and selected and online and same
            and rc_wrong != 0)
    record_live("B3-normal-api", core,
                "State=%s, exit nodes listed: %d, configured exit node selected: %s and online: %s, same set and "
                "same online flags as the system client reports: %s, wrong secret rejected: %s" % (
                    info.get("State"), len(rows), selected, online, same, rc_wrong != 0),
                state=info.get("State"))
    heartbeat()
    path = ts_path(v["exit_node"])
    heartbeat()
    rc_mode, out_mode, _ = api_cli(["mode"])
    clash_mode, _ = get_mode()
    rc_list, out_list, _ = api_cli(["connection", "list", "--columns", "process,outbound"])
    extras = path in ("direct", "relay", "peer-relay") and rc_mode == 0 and out_mode.strip() == clash_mode
    record_live("B4-normal-api-extras", extras,
                "path to the exit node: %s, `api mode` equals GET /configs (%s): %s, `api connection list` rows: %d"
                % (path, clash_mode, out_mode.strip() == clash_mode, len(out_list.splitlines())), path=path)
    heartbeat()


if __name__ == "__main__":
    main()
```

Проверка, что файлы записаны без ошибок:

```powershell
foreach ($f in 'check_hold.py', 'check_modes.py', 'check_rules.py', 'check_revmap.py', 'check_api.py') { & C:\Python314\python.exe -m py_compile "C:\code\router\spike\plan0\$f"; if ($LASTEXITCODE -eq 0) { "compiled $f" } else { "BROKEN $f" } }
```

Ожидается: пять строк `compiled ...`.

Проверка правил записи `K3` (шаг 8) на заданных ответах вместо настоящих пакетов; ядро, сеанс и сеть для неё не нужны:

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\check_revmap.py selftest
```

Ожидается одна строка:

```text
ok   check_revmap: a lost datagram is sent again; FAIL needs a wrong lane or three losses in a row
```

Строка подтверждает пять случаев: все шесть размеров ответили сразу — `PASS` за один запуск `stun.js`; пакет без ответа отправлен снова и получил ответ — `PASS`; два размера остались без ответа три раза подряд при ответивших остальных — `FAIL`; ответ с прямой дорожки — `FAIL` без повторной отправки; без ответа все размеры — оценки нет. При `AssertionError` или `Traceback` шаг 2 не начинается: файл записан с ошибкой, запишите его заново из плана.

- [ ] **Step 2: Получить согласие и запустить удержание (нужен пользователь: согласие, UAC)**

Сначала согласие. В шагах 6, 7 и 10 основная дорожка несколько раз становится прямой, а в режимах с запасным DNS запросы имён идут к провайдеру. Программы исполнителя и тестовые копии закреплены правилами, остальные программы ПК — нет. Передайте пользователю этот текст и дождитесь ответа:

```text
Во время этой проверки несколько раз, отрезками примерно по минуте (всего около 5 минут), все программы компьютера, кроме моих и тестовых, будут выходить в интернет напрямую, без VPN, а запросы имён будут видны провайдеру: так проверяются основная дорожка «Напрямую» и режим «всё напрямую». Закройте на это время программы и вкладки, которые не должны выходить напрямую. Если это неприемлемо, скажите — я пропущу шаги 6 и 10 и часть шага 7; решение по 6.3 тогда останется непринятым.

Кроме того, за время проверки связь у всех программ компьютера 6–8 раз (2–4 раза, если шаги 6 и 10 пропускаются) пропадёт на 5–25 секунд: при каждом включении, перезапуске ядра и откате. Открытые соединения при этом рвутся (звонки, игры, загрузки, фоновые задачи). В значке Tailscale в это время ничего не переключайте: я сам верну всё в конце. Программы, запущенные через python.exe и node.exe, в режимах «напрямую» остаются на VPN: они закреплены вместе с моими.
```

Если пользователь согласен, задача выполняется как написано. Если пользователь не согласен: шаги 6 и 10 пропускаются, шаг 7 выполняется с аргументом `no-direct` (команда приведена в шаге 7), остальные шаги — без изменений. Тогда записи `A2-*`, `A3`, `A4`, `A5`, четыре записи `A1-start-*` (кроме `A1-start-vpn`) и `V5` не появляются, `report.py` печатает `DECISION 1: OPEN`, а в приложении A эти проверки остаются в абзаце «Не проверялось».

Затем предупредите пользователя: сейчас появится запрос UAC от Python; его нужно принять в течение 120 секунд. Отмена ничего не меняет. Запускайте команду только после ответа пользователя, что он готов.

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Start-P0Watchdog -Mode hold -Config p0-hold.json
```

Ожидается (идентификатор запуска будет другим; утром ядро поднималось за 1 секунду, туннель подтверждался за 2–8 секунд):

```text
P0-LAUNCH: run=1a2b3c4d mode=hold config=p0-hold.json - the UAC prompt must be accepted within 120 s
P0-STATUS: state=starting run=1a2b3c4d seq=1 cmd_id= config=p0-hold.json
P0-STATUS: state=holding run=1a2b3c4d seq=2 cmd_id= config=p0-hold.json exit_node_still_on=True
P0-RESULT: state=holding run=1a2b3c4d seq=2 cmd_id= config=p0-hold.json exit_node_still_on=True
```

Другой итог — по таблице «Состояния watchdog и что делать» в Global Constraints. `P0-RESULT: UAC-CANCELLED`: система не изменена; не повторять без согласия пользователя.

Утром такой адаптер работал рядом с включённым системным exit node несколько секунд, сейчас он простоит несколько минут. Если после состояния `holding` связь исполнителя пропадёт, ничего делать не нужно: `deadline.txt` перестанет обновляться, и не позже чем через 400 секунд watchdog сам остановит ядро удержания; системный exit node в этом режиме не трогается.

- [ ] **Step 3: Проверки при удержании: (г) и (б) до переключения**

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_hold.py
```

Ожидается:

```text
PASS G0-hold-system-vpn system exit node on: True, this process still leaves through it: True
PASS G1-hold-probe-vpn probe-vpn left through the exit node 3 of 3 times
PASS G2-hold-probe-direct probe-direct answered 3 of 3 times, with an address other than the VPN one 3 times, distinct addresses: 1
PASS B2-hold-api State=Running, exit nodes listed: 1, configured exit node marked selected: True, path to it: direct
PASS A0-mode-list GET /configs: mode=vpn, mode-list is exactly the five modes: True
```

Критерии:

| Запись | Условие `PASS` |
|---|---|
| `G0-hold-system-vpn` | системный exit node включён, и запрос самого скрипта выходит с адреса VPN: удержание не затронуло программы |
| `G1-hold-probe-vpn` | три запроса из трёх через `probe-vpn` вышли с адреса exit node |
| `G2-hold-probe-direct` | три запроса из трёх через `probe-direct` получили ответ, адрес один и тот же и не равен адресу VPN |
| `B2-hold-api` | состояние `Running`, настроенный exit node помечен `selected`, `ping` до него ответил (`direct`, `relay` или `peer-relay`) |
| `A0-mode-list` | режим `vpn`, в списке ровно пять режимов |

Если `G2-hold-probe-direct` — `FAIL`: это наблюдение об адаптере удержания (он перехватывает один адрес и в продукте не используется), а не итог (г). Выполните шаг 4: итог (г) даёт `G3-engage-probe-direct` — полный адаптер, ровно шаг 5 из 5.3. Работу останавливает только `FAIL G3`.

`B2-hold-api` в решении по разделу 7 не участвует; `FAIL` здесь записывается как наблюдение: на шаге 5 включения служба API ещё не даёт полных данных.

- [ ] **Step 4: Включить деление**

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action engage -Config p0-main-vpn.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_hold.py engaged
```

Ожидается (утром включение занимало 8 секунд, программы были без связи 4–6 секунд):

```text
P0-COMMAND: engage p0-main-vpn.json id=c1a2b3c4
P0-STATUS: state=holding run=1a2b3c4d seq=2 cmd_id= config=p0-hold.json exit_node_still_on=True
P0-STATUS: state=hold_to_engage run=1a2b3c4d seq=3 cmd_id=c1a2b3c4 config=p0-main-vpn.json
P0-STATUS: state=engaging run=1a2b3c4d seq=4 cmd_id=c1a2b3c4 config=p0-main-vpn.json
P0-STATUS: state=ready run=1a2b3c4d seq=5 cmd_id=c1a2b3c4 config=p0-main-vpn.json
P0-RESULT: state=ready run=1a2b3c4d seq=5 cmd_id=c1a2b3c4 config=p0-main-vpn.json
PASS G3-engage-probe-direct watchdog measured probe-direct before switching the system exit node off (full adapter): 1 measurement(s), the last one got an answer: True, from the address the direct lane has after the switch: True
```

`G3-engage-probe-direct` — итог (г): запрос через `probe-direct` при полном адаптере, после доказательства туннеля и до выключения системного exit node, то есть ровно шаг 5 из 5.3. Watchdog повторяет запрос до трёх раз, если ответа нет; в записи — число измерений и итог последнего. Условие `PASS`: последнее измерение получило ответ с того же адреса, с которого прямая дорожка выходит после выключения системного exit node, то есть с адреса провайдера. Этот адрес скрипт измеряет сам через `probe-direct` уже в состоянии `ready`; адреса только сравниваются и не печатаются. Если watchdog ответа не получил, запись `FAIL` делается без этого измерения.

Если вместо записи напечатана строка `ABORT lane addresses not measurable ...` или строка `ABORT probe-vpn ...` (адреса двух дорожек совпали либо адрес дорожки VPN не тот), адрес прямой дорожки после переключения измерить не удалось и запись `G3` не сделана. Команда `reset` из правил задачи здесь не нужна: режим не менялся. Повторите один раз только скрипт:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_hold.py engaged
```

Если `ABORT` повторился, следующие шаги выполнить нельзя (каждый начинается с того же измерения): выполните шаг 12, затем задачу 6; в отчёт записывается текст строки `ABORT`, проверка (г) остаётся в абзаце «Не проверялось».

`FAIL G3` — проверка (г) не пройдена. Выполните шаг 12, затем задачу 6. По спецификации провал (г) останавливает работу и возвращает вопрос владельцу продукта: шаг 5 включения (5.3) не сможет получить адрес прямой дорожки.

Если `P0-RESULT` не `state=ready`, действуйте по таблице состояний в Global Constraints; запись `G3` при этом не появляется (`ABORT SESSION-NOT-READY`).

- [ ] **Step 5: (а) Стартовый режим `vpn` и закрепление программ исполнителя**

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_modes.py start vpn
```

Ожидается:

```text
PIN OK pin rule 8 precedes mode rule 10; python.exe matched it; pinned connections checked: 12
PASS A1-start-vpn core started in vpn: mode read back, unpinned and pinned copies and DNS as designed
LEFT-IN-MODE vpn
```

`PIN OK` — обязательное условие для всех следующих шагов с прямой основной дорожкой: правило 8 (имена процессов) и правило 9 (адреса сервиса исполнителя) стоят перед правилами режимов, запрос `python.exe` попал именно в правило 8, и ни одно соединение `claude.exe` или `bun.exe` из журнала ядра не прошло мимо него. Соединения, владельца которых ядро не определило, в журнале к программе не привязаны и этой проверкой не видны; для них и стоит правило 9. При `PIN FAILED` скрипт сам возвращает режим `vpn` и останавливается с записью `A-pin`; шаги 6 и 10 не выполняются. Шаг 7 выполнить можно: его часть с режимом `direct` скрипт пропустит сам и напечатает `SKIP V5-vpn-file-direct-main not run, pin not confirmed`. Затем шаги 8, 9, 11, 12. Отказ закрепления сообщается владельцу продукта вместе с остальными результатами: `report.py` печатает `A-pin` в строке `OWNER-QUESTION`.

Что проверяет `A1-start-<режим>` для каждого режима:

| Режим | Незакреплённая копия `curl-free.exe` | Копия из файла VPN | Копия из прямого файла | Сервер DNS |
|---|---|---|---|---|
| `vpn` | VPN | VPN | DIRECT | итоговый (`dns-vpn`) |
| `vpn-localdns` | VPN | VPN | DIRECT | `dns-local` |
| `direct` | DIRECT | VPN | DIRECT | итоговый (`dns-vpn`) |
| `direct-localdns` | DIRECT | VPN | DIRECT | `dns-local` |
| `alldirect` | DIRECT | DIRECT | DIRECT | `dns-local` |

Дорожка определяется по адресу, с которого сервис `api.ipify.org` видит запрос копии. Сервер DNS определяется так: скрипт отправляет запрос имени `www.example.com` прямо на адрес DNS адаптера `172.19.0.2` и ищет в журнале ядра строку `dns: match[...] clash_mode=<режим> => route(dns-local)` для этого запроса; её отсутствие при полученном ответе означает итоговый сервер.

- [ ] **Step 6: (а) Все двадцать переходов**

Шаг выполняется только после `PIN OK` и только если пользователь дал согласие в шаге 2. Примерно минуту основная дорожка побывает прямой: незакреплённые программы ПК в это время выходят напрямую. После двадцати переходов скрипт проверяет четвёртое свойство из 6.3: что смена режима делает с открытым соединением.

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_modes.py euler
```

Ожидается:

```text
PIN OK pin rule 8 precedes mode rule 10; python.exe matched it; pinned connections checked: 14
  vpn              -> vpn-localdns     OK
  vpn-localdns     -> vpn              OK
  vpn              -> direct           OK
  direct           -> vpn              OK
  vpn              -> direct-localdns  OK
  direct-localdns  -> vpn              OK
  vpn              -> alldirect        OK
  alldirect        -> vpn-localdns     OK
  vpn-localdns     -> direct           OK
  direct           -> vpn-localdns     OK
  vpn-localdns     -> direct-localdns  OK
  direct-localdns  -> vpn-localdns     OK
  vpn-localdns     -> alldirect        OK
  alldirect        -> direct           OK
  direct           -> direct-localdns  OK
  direct-localdns  -> direct           OK
  direct           -> alldirect        OK
  alldirect        -> direct-localdns  OK
  direct-localdns  -> alldirect        OK
  alldirect        -> vpn              OK
PASS A2-transitions-all-pairs 20 ordered transitions, failures: 0, retries: 0
PASS A3-unknown-mode unknown mode: PATCH answered HTTP 204, mode before vpn, after vpn
INFO A4-mode-case PATCH with VPN-LOCALDNS: HTTP 204, mode read back as vpn-localdns
PASS A5-open-connections vpn->alldirect: opened on vpn, after the switch still on vpn, new request follows the new mode: True, closed by id: True; alldirect->vpn: opened on direct, after the switch still on direct, new request follows the new mode: True, closed by id: True
LEFT-IN-MODE vpn
```

Критерии. Переход засчитан, если `PATCH /configs` ответил 204, `GET /configs` вернул новый режим, незакреплённая копия вышла с адреса основной дорожки этого режима и запрос DNS обработал нужный сервер и получил ответ; при первом входе в режим дополнительно проверяются обе закреплённые копии. `A2-transitions-all-pairs` — `PASS`, если засчитаны все двадцать. `A3-unknown-mode` — `PASS`, если на неизвестный режим API ответил 204, а режим не изменился. Число `retries` — сколько раз копия сначала ответила не с той дорожки; оно записывается как наблюдение о том, как часто ядро не определяет владельца соединения (правило «два раза подряд» из 5.3). Повтор запроса, оставшегося без ответа, в это число не входит.

`A5-open-connections`. Копия `node-vpn.exe` открывает соединение TLS с `example.org:443` и держит его 20 секунд; режим переключается, через полторы секунды скрипт смотрит, на каком выходе осталось это соединение в `GET /connections`, отправляет новый запрос копией `curl-pin-vpn.exe` и закрывает открытое соединение запросом `DELETE /connections/<id>`. Опыт идёт в обе стороны: `vpn` → `alldirect` и `alldirect` → `vpn`; второй случай — соединение программы дорожки VPN, открытое напрямую, из 8.3. Условие `PASS` в обоих направлениях: соединение осталось на прежнем выходе, новый запрос пошёл по новому режиму, после `DELETE` соединения в списке нет. Если открытое соединение не появилось в списке за 12 секунд, направление не измерено; при прочих успешных — запись `INFO`.

`A5` в решение 1 не входит. Текст `still on None` означает, что ядро само закрыло соединение при смене режима: расхождение с 6.3, записывается в отчёт; сверке 8.3 это не мешает. Текст `closed by id: False` означает, что сверка 8.3 этим способом невыполнима: вопрос владельцу продукта, `report.py` печатает `A5-open-connections` в строке `OWNER-QUESTION`.

Если `A2-transitions-all-pairs` — `FAIL` и в его тексте есть причины кроме `dns-no-answer`, шаг 10 не выполняется: решение по 6.3 уже определено как запасной способ. `FAIL` только с `dns-no-answer` — по правилу из начала задачи: шаг повторяется один раз. Если после повтора причина по-прежнему только `dns-no-answer`, шаг 10 выполняется: решение 1 принимается по его записям.

- [ ] **Step 7: (в) Правила по пути процесса**

Если пользователь дал согласие в шаге 2:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_rules.py
```

Если пользователь не дал согласия, та же проверка без под-шага в режиме `direct`:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_rules.py no-direct
```

Во втором случае вместо строки `V5` печатается `SKIP V5-vpn-file-direct-main not run: the user did not allow the direct main lane`, записи `V5` нет, остальной вывод тот же.

Ожидается (секунды в `V6-hot-reload` будут другими; спецификация ждёт около секунды):

```text
PASS V0-control space=VPN, brackets=VPN, cyr=VPN, outer=VPN, inner=VPN
PASS V1-space-folder space=DIRECT
PASS V2-brackets-file brackets=DIRECT
PASS V3-cyrillic-file cyr=DIRECT
PASS V9-inline-regex inline=DIRECT (rule in the config, not in a file)
PASS V4-nested-invert outer=DIRECT, inner=VPN
PASS V5-vpn-file-direct-main pin-vpn=VPN, inner=VPN; unpinned copy=DIRECT
PASS V6-hot-reload seconds from the write to the start of the first request that follows the rewritten file (add, remove, add): [0.9, 1.0, 0.7]; limit 3 s
PASS V7-system-path :System connections seen: 1, matched by the VPN lane file: 1
PASS V8-broken-file error line in the core log: True; previous rules stayed in force: True; line: reload rule-set lane-direct: parse rule_set.rules.[7]: process_path_regex: parse expression 0: error parsing regexp: missing closing ): `(?i)^C:\\broken(`
RULES-RESTORED base rule files written
LEFT-IN-MODE vpn
```

Что проверяет каждая запись:

| Запись | Опыт | Условие `PASS` |
|---|---|---|
| `V0-control` | с базовыми файлами правил проверяемые копии не подпадают ни под одно правило | все пять копий выходят через VPN |
| `V1-space-folder` | правило папки `...\Dir With Spaces\` в прямом файле | копия выходит напрямую |
| `V2-brackets-file` | правило файла в папке `Brackets (x86) [v1]` | копия выходит напрямую |
| `V3-cyrillic-file` | правило файла с кириллицей в имени папки и файла | копия выходит напрямую |
| `V9-inline-regex` | правило 6 основного конфига (`process_path_regex` в самом конфиге) | копия `curl-inline.exe` выходит напрямую |
| `V4-nested-invert` | папка `nest\outer\` — напрямую, файл `nest\outer\inner\curl-inner.exe` — через VPN; в прямом файле логическое правило «папка И НЕ файл» | `curl-outer.exe` — напрямую, `curl-inner.exe` — через VPN |
| `V5-vpn-file-direct-main` | режим `direct` на время нескольких запросов; сначала незакреплённая копия подтверждает, что основная дорожка стала прямой | копии из файла VPN (`curl-pin-vpn.exe` и вложенная `curl-inner.exe`) выходят через VPN. Если режим `direct` не включился или незакреплённая копия не вышла напрямую, печатается `SKIP` и записи нет: это провал (а), а не (в). Вложенная копия оценивается, только если `V4` — `PASS` |
| `V6-hot-reload` | файл переписан атомарно три раза | первый запрос, который пошёл по новому правилу, начат не позже чем через 3 секунды после записи файла, каждый раз (время самого запроса в счёт не входит) |
| `V7-system-path` | обращение к папке `\\192.0.2.1\p0` (адрес для документации, никуда не ведёт) заставляет ядро Windows открыть соединение от имени процесса 4 | соединение с путём `:System` попало под файл дорожки VPN |
| `V8-broken-file` | в прямой файл добавлено выражение с ошибкой | в журнале строка `reload rule-set lane-direct`, прежние правила действуют |

Что это значит для плана 1:

- `FAIL` в `V1`, `V2`, `V3`, `V5`, `V6`, `V9` или `V7` (соединение `:System` было, но под правило не попало) — провал (в) по `process_path_regex`. По спецификации это останавливает работу и возвращает вопрос владельцу продукта. Доведите задачу 3 до конца (шаги 8, 9, 12) и переходите к задаче 6.
- Строка `SKIP V5-...` (нет согласия пользователя или не подтверждено закрепление) — не провал: под-шаг не выполнялся, проверка остаётся в абзаце «Не проверялось».
- Строка `SKIP V5-... mode direct did not give a direct main lane` — режим `direct` не дал прямой основной дорожки: это провал (а) (решение 1 — запасной способ), а не (в). Запись `V5` не появляется, проверка остаётся в абзаце «Не проверялось» и повторяется в плане 1 уже в запасной схеме 6.3.
- `PASS V5-...` с текстом `nested copy not judged: V4 failed` — файл дорожки VPN при прямой основной дорожке работает (копия `curl-pin-vpn.exe`), а вложенная копия не оценивалась: при неработающем логическом правиле её в любом режиме забирает прямой файл, и это решение 3 (8.2), а не провал (в).
- `FAIL V0-control` вместе с `ABORT MEASUREMENT-FAILED` — контроль не прошёл: с базовыми файлами правил тестовая копия вышла не по основной дорожке, записи `V1`–`V9` не сделаны. После команды `reset` из правил задачи подождите 5 секунд и повторите шаг один раз. Если контроль не прошёл и при повторе, выполните шаги 8, 9, 12 и задачу 6: проверки (в) остаются в абзаце «Не проверялось», в отчёт записывается текст строки `FAIL V0-control`.
- `FAIL V6-hot-reload`, в тексте которого все три времени — числа (перечитывание состоялось, но один из запросов начался позже 3 секунд): выполните команду `reset` и повторите шаг 7 один раз. Провалом (в) считается только повторный `FAIL` или значение `None` среди времён.
- `INFO V7-system-path` означает, что соединение от имени `:System` создать не удалось; псевдопуть остаётся непроверенным, работу это не останавливает.
- `FAIL` только в `V4-nested-invert` — решение по 8.2: запасной способ (правила 9 и 10 в 6.2 меняются местами, раскладка с правилом прямой дорожки внутри более широкого правила VPN отклоняется).
- `FAIL V6-hot-reload` с текстом `rename over the file was not applied` и `rewriting in place applied: True` — атомарная замена файла не замечается ядром; это противоречит 6.5 и возвращается владельцу продукта вместе с остальными результатами.
- `FAIL V8-broken-file` работу не останавливает: план 1 не сможет опираться на строку `reload rule-set` и должен полагаться только на сверку соединений (8.3).
- Запись `V1-reload-error` появляется, только если ядро не смогло прочитать исправный файл правил первого этапа; в ней текст ошибки из журнала. Тогда `V1`–`V3` не говорят о `process_path_regex` ничего: сначала нужно понять причину ошибки.

- [ ] **Step 8: (к) Имена узлов и путь без разбора протоколов**

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_revmap.py
```

Ожидается:

```text
PASS K1-host-names connection list shows the host name for the VPN lane copy: True, for the direct lane copy: True; each on its own outbound: True; core log names the mapping: True
INFO K5-host-alias for a connection to the alias www.wikipedia.org the list shows: dyna.wikimedia.org (not the name asked for)
PASS K2-tcp-lanes pinned copies leave through their lanes
PASS K3-udp-vpn-sizes STUN through the VPN lane, payload size -> lane: {'20': 'VPN', '624': 'VPN', '1124': 'VPN', '1224': 'VPN', '1324': 'VPN', '1424': 'VPN'}
PASS K4-udp-direct STUN from the direct lane copy, two servers: ['DIRECT', 'DIRECT']
```

Критерии:

| Запись | Опыт | Условие `PASS` |
|---|---|---|
| `K1-host-names` | после сброса кэша DNS Windows копии `node-vpn.exe` и `node-direct.exe` открывают соединение с `example.org:443` и держат его 15 секунд | в `GET /connections` у обоих соединений `metadata.host` равен `example.org`, а первый элемент `chains` — `vpn` и `direct`; если соединение копии не появилось в списке за 12 секунд — `ABORT MEASUREMENT-FAILED`, шаг повторяется |
| `K5-host-alias` | то же из `node-vpn.exe` для имени-псевдонима `www.wikipedia.org` (запись CNAME; скрипт сначала спрашивает у системы каноническое имя) | оценки нет: записывается, какое имя показано. Если имя перестало быть псевдонимом или соединение не появилось в списке — строка `SKIP`, записи нет |
| `K2-tcp-lanes` | закреплённые копии `curl` | каждая выходит через свою дорожку |
| `K3-udp-vpn-sizes` | `spike\scripts\stun.js` из копии `node-vpn.exe`, шесть размеров пакета от 20 до 1424 байт. Пакет, на который за 4 секунды не пришёл ответ, отправляется снова: до трёх попыток на размер | для каждого размера сервер STUN видит адрес VPN, пусть и не с первой попытки. `FAIL`: ответ пришёл с другой дорожки, либо часть размеров осталась без ответа все три раза, а остальные ответили (так выглядела потеря больших пакетов при разборе протоколов, спецификация 6.1). Если без ответа остались все шесть размеров, оценивать нечего: `ABORT MEASUREMENT-FAILED`, записи `K3` нет |
| `K4-udp-direct` | тот же скрипт из `node-direct.exe`, два сервера, пакет 20 байт, те же три попытки | хотя бы один сервер видит прямой адрес, ни один не видит адрес VPN; без ответов — `INFO` |

Один потерянный пакет записью `FAIL` не становится. Если пакет получил ответ только при повторной отправке, запись остаётся `PASS`, а в её тексте добавляется, например, `; answered only when sent again: [1324]`. В записи `FAIL` из-за потерь текст заканчивается перечнем размеров, например `; no answer in 3 attempts: [1324, 1424]`.

Что это значит для плана 1:

- `K1-host-names` — `FAIL`: настройка `dns.reverse_mapping` выключается, в соединениях показываются адреса (спецификация, раздел 12).
- `K5-host-alias` с текстом `not the name asked for` — ожидаемый исход: ядро запоминает имя записи A, а для имени-псевдонима это каноническое имя из конца цепочки CNAME (в день подготовки плана `www.wikipedia.org` указывало на `dyna.wikimedia.org`). Для `example.org` запись A принадлежит самому имени, поэтому `K1` от этого не зависит. В списке соединений (8.4) у части сайтов будет показано имя сети доставки, а не сайта; это записывается в отчёт для владельца продукта и работу не останавливает. Текст `the name asked for` означал бы обратное: для псевдонима показано запрошенное имя.
- Строка `SKIP K5-host-alias ...` — имя `www.wikipedia.org` перестало быть псевдонимом либо соединение не появилось в списке: наблюдения нет, строки о цепочке CNAME в приложении A не будет. Ни одно решение от `K5` не зависит.
- `K3-udp-vpn-sizes` — `FAIL`: по одному прогону вывод о `dns.reverse_mapping` не делается. Порядок действий — ниже, в абзаце «Если `K3` — `FAIL`».
- Записи `K3-udp-vpn-sizes` нет, скрипт закончился строкой `ABORT MEASUREMENT-FAILED` — ниже, в абзаце «Если по дорожке VPN не ответил ни один размер».
- `K4-udp-direct` — `FAIL` (копия прямой дорожки вышла через VPN) записывается как расхождение с приложением A.

**Если `K3` — `FAIL`.** Нужны ещё два прогона той же проверки UDP: контрольный, с выключенным обратным сопоставлением (запись `K3n`), и повторный, снова с основным конфигом (запись `K3` заменяется; картина первого прогона остаётся в ней в поле `first_run`). Настройка выключается, только если провал исчезает без обратного сопоставления и возвращается вместе с ним. Перед первой командой узнайте возраст сеанса командой `Get-P0Session` из правил задачи: если `engaged_minutes` не меньше 35, сначала шаг 12 и новый сеанс; запись `K3` первого прогона хранится в `results.json` и при этом не пропадает. Обе команды — инструмент PowerShell, тайм-аут 600000 мс. Каждая перезапускает ядро: программы без связи около 15 секунд.

Контрольный прогон:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action restart -Config p0-main-vpn-norevmap.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_revmap.py udp
```

Ожидается `P0-RESULT: state=ready ... config=p0-main-vpn-norevmap.json`, затем строки `K3n-udp-vpn-sizes` (в конце текста `same picture as with reverse_mapping on: True` или `False`) и `K4n-udp-direct`.

Повторный прогон. Команда выполняется при любом исходе контрольного прогона, потому что она же возвращает основной конфиг:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action restart -Config p0-main-vpn.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_revmap.py again
```

Ожидается `P0-RESULT: state=ready ... config=p0-main-vpn.json`, затем строки `K3-udp-vpn-sizes` (в конце текста `same picture as in the first run: True` или `False`) и `K4-udp-direct`. Пример, когда провал повторился:

```text
FAIL K3-udp-vpn-sizes STUN through the VPN lane, payload size -> lane: {'20': 'VPN', '624': 'VPN', '1124': 'VPN', '1224': 'VPN', '1324': 'NONE', '1424': 'NONE'}; no answer in 3 attempts: [1324, 1424]; same picture as in the first run: True
PASS K4-udp-direct STUN from the direct lane copy, two servers: ['DIRECT', 'DIRECT']
```

Вывод делается по двум записям:

| `K3n` (сопоставление выключено) | `K3` после повторного прогона (сопоставление включено) | Вывод |
|---|---|---|
| `PASS` | `FAIL` | провал исчез без обратного сопоставления и вернулся вместе с ним: оно ломает путь без разбора протоколов. Настройка `dns.reverse_mapping` выключается, в соединениях показываются адреса (спецификация, раздел 12); вывод записывается в отчёт одной фразой (задача 6, шаг 4) |
| `PASS` | `PASS` | провал первого прогона не повторился: это были потери пакетов, а не свойство ядра. Настройка остаётся включённой; первый прогон записывается в отчёт одной фразой (задача 6, шаг 4) |
| `FAIL` | `FAIL` | обратное сопоставление ни при чём: расхождение с приложением A (большие пакеты UDP по дорожке VPN) записывается в отчёт и возвращается владельцу продукта, настройка остаётся включённой |
| `FAIL` | `PASS` | потери не следуют за настройкой: путь UDP нестабилен. Настройка остаётся включённой; наблюдение записывается в отчёт, работу оно не останавливает |

Если скрипт печатает `ABORT SESSION-NOT-READY: ... needs the core started with ...`, перезапуск не состоялся и ядро работает не с тем конфигом (в строке `P0-RESULT` стоит `cmd_error=...`, либо состояние не `ready`); записи не сделаны. Действуйте по таблице состояний в Global Constraints и повторите команду. К шагу 9 переходите только после `P0-RESULT: state=ready ... config=p0-main-vpn.json`.

**Если по дорожке VPN не ответил ни один размер.** Строка `ABORT MEASUREMENT-FAILED: the STUN server answered none of the 6 datagram sizes ...` может появиться в любом из трёх прогонов. Запись `K3` (или `K3n`) этого прогона не сделана, запись `K4` (или `K4n`) сделана. Выполните команду `reset` из правил задачи и один раз повторите тот же вызов скрипта без перезапуска ядра. Для первого прогона это команда из начала шага, для контрольного и повторного — соответственно:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_revmap.py udp
```

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_revmap.py again
```

Если `ABORT` повторился:

- в первом прогоне — записи `K3` нет, в приложении A эта проверка остаётся со словами «не проверялось», контрольный и повторный прогоны не выполняются. В строке `ABORT` сказано, ответил ли тот же сервер копии прямой дорожки. `True`: сервер доступен, а по дорожке VPN пакеты UDP до него не проходят совсем; это расхождение с приложением A, оно записывается в отчёт (задача 6, шаг 4) и возвращается владельцу продукта. `False`: сервер STUN недоступен и проверка не состоялась; в отчёт записывается именно это. Настройка `dns.reverse_mapping` в обоих случаях определяется по `K1`;
- в контрольном или повторном прогоне — таблица исходов не читается, настройка остаётся включённой. После несостоявшегося контрольного прогона команда повторного прогона всё равно выполняется: она возвращает основной конфиг. Если запись `K3` в итоге осталась `FAIL`, расхождение с приложением A записывается в отчёт и возвращается владельцу продукта. Если повторный прогон записал `PASS`, первый прогон записывается в отчёт одной фразой (задача 6, шаг 4).

- [ ] **Step 9: (б) Служба API в обычном режиме**

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_api.py
```

Ожидается:

```text
PASS B3-normal-api State=Running, exit nodes listed: 1, configured exit node selected: True and online: True, same set and same online flags as the system client reports: True, wrong secret rejected: True
PASS B4-normal-api-extras path to the exit node: direct, `api mode` equals GET /configs (vpn): True, `api connection list` rows: 37
```

`B3-normal-api` — то, что раздел 7 требует от основного источника данных. Условие `PASS`: состояние `Running` без ссылки для входа; настроенный exit node есть в списке, помечен `selected` и «в сети»; список совпадает с тем, что системный клиент считает exit node (те же адреса и те же признаки «в сети»; сравнение повторяется до трёх раз с паузой 2 секунды); неверный секрет отклонён. `FAIL` — решение по разделу 7: запасной способ.

`B4-normal-api-extras` — дополнения, без которых раздел 7 обходится. Условие `PASS`: `ping` до exit node ответил (`direct`, `relay` или `peer-relay`), `sing-box api mode` совпал с `GET /configs`. Значение `path` — источник для поля `tailscale.path` из 5.2. `B4` — `FAIL`: служба API остаётся основным источником; `tailscale.path` в плане 1 равен `null` (как в запасном способе раздела 7), режим ядра читается только через Clash API. В решение 2 запись `B4` не входит.

- [ ] **Step 10: (а) Остальные стартовые режимы**

Условия: пользователь дал согласие в шаге 2, шаг 5 напечатал `PIN OK`, шаг 6 — `PASS A2-transitions-all-pairs` либо `FAIL`, единственная причина которого и после одного повтора — `dns-no-answer`. Перед шагом:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Get-P0Session
```

Ожидается `P0-SESSION: state=ready ... engaged_minutes=<меньше 30> (hard cap 45)`. Если минут 30 или больше — сначала шаг 12 и новый сеанс, как сказано в начале задачи.

Четыре команды, по одной на вызов инструмента PowerShell с тайм-аутом 600000 мс. Каждая перезапускает ядро с другим `default_mode` и сразу запускает проверку; проверка заканчивается в режиме `vpn`. При перезапуске watchdog сначала возвращает и подтверждает системный exit node, затем останавливает ядро и включает новое; программы без связи около 15 секунд. С конфигами `direct`, `direct-localdns` и `alldirect` незакреплённые программы ПК выходят напрямую с момента включения до конца проверки, около минуты.

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action restart -Config p0-main-vpn-localdns.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_modes.py start vpn-localdns
```

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action restart -Config p0-main-direct.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_modes.py start direct
```

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action restart -Config p0-main-direct-localdns.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_modes.py start direct-localdns
```

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action restart -Config p0-main-alldirect.json; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\check_modes.py start alldirect
```

Ожидается для каждой (пример для `direct`):

```text
P0-COMMAND: restart p0-main-direct.json id=c5d6e7f8
P0-STATUS: state=restarting run=1a2b3c4d seq=9 cmd_id=c5d6e7f8 config=p0-main-direct.json
P0-STATUS: state=ready run=1a2b3c4d seq=10 cmd_id=c5d6e7f8 config=p0-main-direct.json
P0-RESULT: state=ready run=1a2b3c4d seq=10 cmd_id=c5d6e7f8 config=p0-main-direct.json
PIN OK pin rule 8 precedes mode rule 10; python.exe matched it; pinned connections checked: 3
PASS A1-start-direct core started in direct: mode read back, unpinned and pinned copies and DNS as designed
  direct           -> vpn              OK
  vpn              -> direct           OK
  direct           -> vpn-localdns     OK
  vpn-localdns     -> direct           OK
  direct           -> direct-localdns  OK
  direct-localdns  -> direct           OK
  direct           -> alldirect        OK
  alldirect        -> direct           OK
PASS A2-transitions-from-direct 8 transitions from direct to every other mode and back, failures: 0, retries: 0
LEFT-IN-MODE vpn
```

Критерии: `A1-start-<режим>` — `PASS`, если сразу после запуска, без единого `PATCH`, `GET /configs` вернул этот режим и таблица из шага 5 выполняется; `A2-transitions-from-<режим>` — `PASS`, если засчитаны все восемь переходов.

Если `P0-RESULT` не `state=ready`, проверка напечатает `ABORT SESSION-NOT-READY`; действуйте по таблице состояний в Global Constraints. Любой другой `ABORT` и `Traceback` — по правилам из начала задачи: ядро только что запущено в режиме с прямой основной дорожкой, поэтому команда `reset` выполняется сразу. Скрипт и сам возвращает режим `vpn` при любом исходе: всё после проверки сеанса выполняется внутри одного `try/finally`, последняя строка вывода — `LEFT-IN-MODE vpn`.

Решение по 6.3: основной способ (`clash_mode`), если `PASS` у `A0-mode-list`, пяти записей `A1-start-*`, `A2-transitions-all-pairs` и четырёх записей `A2-transitions-from-*`. Любой `FAIL` среди них — запасной способ из 6.3; исключение — `FAIL` только с причиной `dns-no-answer` (правила задачи). Запасной способ согласован в спецификации и ответа владельца продукта не ждёт. `A3-unknown-mode` — `FAIL` в решение 1 не входит: служебный процесс отправляет только пять известных режимов и после каждого переключения читает `GET /configs`, поэтому схема 6.3 от ответа API на неизвестный режим не зависит. Такой `FAIL` записывается в отчёт как расхождение с текстом 6.3; чтение `GET /configs` после переключения остаётся обязательным в любом случае.

- [ ] **Step 11: Сохранить журналы сеанса**

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Copy-Item C:\code\router\spike\run\phase-b.log C:\code\router\spike\plan0\out\session-core-last.log -Force; Copy-Item C:\code\router\spike\run\watchdog.log C:\code\router\spike\plan0\out\session-watchdog.log -Force; Get-P0Session
```

Ожидается: `P0-SESSION: state=ready ...`. Журналы остаются в `spike\plan0\out\` и в git не попадают.

- [ ] **Step 12: Закончить сеанс и проверить систему**

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action stop
```

Ожидается (утром откат занимал 1–3 секунды; если сеанс заканчивается из удержания, итоговое состояние — `hold_done` с `reason=stop command`; если сеанс уже откатился сам, печатается `P0-RESULT: NO-LIVE-SESSION`, это нормально):

```text
P0-COMMAND: stop  id=c9a8b7c6
P0-STATUS: state=rolling_back run=1a2b3c4d seq=18 cmd_id=c9a8b7c6 reason=stop command
P0-STATUS: state=rolled_back run=1a2b3c4d seq=19 cmd_id=c9a8b7c6 reason=stop command system_vpn_ok=True
P0-RESULT: state=rolled_back run=1a2b3c4d seq=19 cmd_id=c9a8b7c6 reason=stop command system_vpn_ok=True
```

Затем проверка системы:

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\prepare.py
```

Ожидается `PREPARE-OK` в конце и строки `STATE ... OK` перед ним: системный exit node включён, `sing-box.exe` не запущен, запрос выходит через VPN. При `state=restore_unverified` или строке `WRONG` покажите пользователю ручное восстановление из Global Constraints. Строка `NO-LIVE-SESSION - stale status` (ПК перезагружался) — по таблице состояний в Global Constraints.

---

### Task 4: (з) Повторный вход после истечения ключа — нужен пользователь

Задача требует действий пользователя в консоли Tailscale; её целиком выполняет исполнитель, который сам ведёт разговор с пользователем. Её провал или пропуск не мешает плану 1: по спецификации (раздел 7) при истёкшем входе деление выключается и режим настройки запускается заново, то есть ядро в любом случае перезапускается. Задача отвечает на вопрос, как именно появляется новая ссылка, и попутно проверяет завершение входа в режиме настройки для (д).

Связь исполнителя в этой задаче не прерывается: системный VPN не трогается, ядро работает без адаптера и без прав администратора, UAC не нужен.

После задачи встроенный узел должен быть снова в сети. Если повторный вход не удался, узел остаётся без входа; задача 5 в этом случае не начинается (проверка в её шаге 2), а вход повторяется этой же задачей.

**Files:**
- Create: `C:\code\router\spike\plan0\check_relogin.py`

**Interfaces:**
- Consumes: из задачи 1 — `p0lib.py` (`require_no_session`, `core_running`, `ts_status`, `ts_exit_nodes`, `api_cli`, `LogTail`, `record`, `SETUP_LOG`), файл `spike\plan0\out\p0-setup.json`.
- Produces: записи `Z0-baseline`, `Z1-expiry-reported`, `Z2-link-without-restart`, `Z3-link-after-restart`, при необходимости `Z3-link-after-logout`, `Z5-relogin`.

- [ ] **Step 1: Записать `check_relogin.py`**

Файл `C:\code\router\spike\plan0\check_relogin.py`, целиком:

```python
"""Plan 0, check (z): what the embedded node does when its key is expired, and how a new login link appears.

The core runs in setup mode (no adapter, not elevated, started in the background by the executor).
usage: check_relogin.py baseline           the node is logged in before the test
       check_relogin.py wait-expired       wait (up to 9 min) until the API service reports the expiry
       check_relogin.py wait-link <label>  wait (up to 60 s) for a login link; label: restart | logout
       check_relogin.py logout             ask the core to log out (last resort to get a link)
       check_relogin.py wait-login         wait (up to 9 min) until the node is logged in again
The login link is printed for the user and is never written to the results.
"""
from p0lib import *


def need_setup_core():
    require_no_session()
    end = time.time() + 20
    while not core_running():
        if time.time() > end:
            die("the setup-mode core is not running")
        time.sleep(1)


def link_in_log(tail):
    return bool(re.search(r"Waiting for authentication: https://\S+", tail.text()))


def baseline():
    # Right after the start the API may not listen yet and the node may report NoState: wait for a settled state.
    info = {}
    end = time.time() + 60
    while time.time() < end:
        info = ts_status()
        if info.get("State") in ("Running", "Starting", "NeedsLogin") or "Log in" in info:
            break
        time.sleep(2)
    state = info.get("State")
    record("Z0-baseline", state in ("Running", "Starting") and "Log in" not in info,
           "before the test: State=%s, login link offered: %s" % (state, "Log in" in info), state=state)


def wait_expired():
    tail = LogTail(SETUP_LOG)
    states, last = [], None
    end = time.time() + 540
    info = {}
    while time.time() < end:
        info = ts_status()
        state = info.get("State")
        if state != last:
            states.append(state)
            last = state
            print("  State=%s" % state)
        if state == "NeedsLogin" or "Log in" in info:
            break
        time.sleep(5)
    expired = info.get("State") == "NeedsLogin" or "Log in" in info
    record("Z1-expiry-reported", expired, "states seen through the API service: %s" % states, states=states)
    if not expired:
        return
    link = False
    end = time.time() + 30
    while time.time() < end and not link:
        info = ts_status()
        link = "Log in" in info or link_in_log(tail)
        time.sleep(3)
    record("Z2-link-without-restart", None,
           "30 s after the expiry, without restarting the core: link from the API service: %s, link in the log: %s" % (
               "Log in" in info, link_in_log(tail)), link=link)
    if "Log in" in info:
        print("LOGIN-LINK " + info["Log in"])


def wait_link(label):
    tail = LogTail(SETUP_LOG)
    tail.pos = 0
    info = {}
    end = time.time() + 60
    while time.time() < end:
        info = ts_status()
        if "Log in" in info:
            break
        time.sleep(2)
    record("Z3-link-after-%s" % label, "Log in" in info,
           "after %s: link from the API service: %s, link in the log: %s, State=%s" % (
               label, "Log in" in info, link_in_log(tail), info.get("State")), state=info.get("State"))
    if "Log in" in info:
        print("LOGIN-LINK " + info["Log in"])


def logout():
    rc, out, err = api_cli(["tailscale", "logout"], timeout=60)
    print("LOGOUT exit code %s" % rc)


def wait_login():
    states, last = [], None
    info = {}
    end = time.time() + 540
    while time.time() < end:
        info = ts_status()
        state = info.get("State")
        if state != last:
            states.append(state)
            last = state
            print("  State=%s" % state)
        if state in ("Running", "Starting") and "Log in" not in info:
            break
        time.sleep(5)
    done = info.get("State") in ("Running", "Starting") and "Log in" not in info
    _, rows = ts_exit_nodes()
    record("Z5-relogin", done, "login through the link in setup mode with the system exit node on: %s; "
           "states: %s; exit nodes listed afterwards: %d" % (done, states, len(rows)), states=states)


if __name__ == "__main__":
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    need_setup_core()
    if command == "baseline":
        baseline()
    elif command == "wait-expired":
        wait_expired()
    elif command == "wait-link" and len(sys.argv) > 2 and sys.argv[2] in ("restart", "logout"):
        wait_link(sys.argv[2])
    elif command == "logout":
        logout()
    elif command == "wait-login":
        wait_login()
    else:
        die("usage: check_relogin.py baseline | wait-expired | wait-link restart|logout | wait-login")
```

- [ ] **Step 2: Запустить ядро в режиме настройки в фоне**

Сначала убедиться, что сеанса watchdog нет и ядро не запущено:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Format-P0Status (Read-P0Status); if (Get-Process -Name sing-box -ErrorAction SilentlyContinue) { 'CORE-RUNNING' } else { 'NO-CORE' }
```

Ожидается: состояние из списка `rolled_back`, `hold_done`, `hold_failed`, `failed_no_change`, `preflight_ok`, `preflight_failed`, `restore_unverified` (только с `reason=stale status closed`) и строка `NO-CORE`. Другое состояние или `CORE-RUNNING` — остановиться: сеанс задачи 3 не закончен, вернитесь к её шагу 12.

Затем запустить ядро. Инструмент PowerShell с параметром `run_in_background: true` (человеку — отдельное окно PowerShell без прав администратора, которое остаётся открытым):

```powershell
Remove-Item -LiteralPath C:\code\router\spike\plan0\out\setup.log -ErrorAction SilentlyContinue; & 'C:\code\router\spike\dl\sing-box-1.14.4\sing-box-1.14.4-windows-amd64\sing-box.exe' run -c C:\code\router\spike\plan0\out\p0-setup.json -D C:\code\router\spike\run
```

Сразу после запуска (скрипт сам ждёт ядро до 20 секунд и состояние узла до 60 секунд):

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\check_relogin.py baseline
```

Ожидается: `PASS Z0-baseline before the test: State=Starting, login link offered: False` (или `State=Running`).

`FAIL` с `State=NeedsLogin` или с `login link offered: True` — узел уже без входа: шаг 3 пропускается, переходите к шагу 5 с командой `wait-link restart`.

`FAIL` с любым другим состоянием (`None`, `NoState`, `Stopped`) — ядро не ответило: остановите его командой из шага 7, прочитайте `spike\plan0\out\setup.log` и повторите шаг 2. К шагу 5 и к команде `logout` при таком исходе не переходить: узел, скорее всего, в порядке, а выход создал бы в сети второе устройство.

- [ ] **Step 3: Прекратить действие ключа узла (нужен пользователь)**

Узнайте имя встроенного узла и сообщите его пользователю в чате; в файлы плана и результатов оно не записывается:

```powershell
(Get-Content -LiteralPath C:\code\router\spike\run\phase-b-v2.json -Raw | ConvertFrom-Json).endpoints[0].hostname
```

Способ один — запрос к API Tailscale. Пункта «прекратить ключ сейчас» в документации консоли нет: страница о сроке действия ключей описывает только отключение срока и временное продление; запрос `POST /api/v2/device/{id}/expire` в API есть. Команды ниже сами находят устройство по точному имени и ничего не прекращают, если устройств с таким именем не ровно одно: так нельзя по ошибке прекратить ключ другого устройства. Действие руками в списке устройств консоли пользователю не предлагается: там такой защиты нет, а в одном списке со встроенным узлом стоят этот компьютер и сервер VPN. Передайте пользователю этот текст:

```text
Нужно досрочно прекратить действие ключа одного устройства — встроенного узла с именем, которое я назвал.

Важно: в том же списке устройств есть сам этот компьютер и сервер, через который идёт VPN (exit node). Их ключи не трогайте: если прекратить ключ одного из них, VPN перестанет работать у всех программ, и вернуть его можно будет только повторным входом на том устройстве.

1. Откройте https://login.tailscale.com/admin/settings/keys и создайте ключ доступа к API (Generate access token) со сроком действия 1 день.
2. Откройте своё окно PowerShell (обычное, без прав администратора), вставьте команды, которые я пришлю следующим сообщением, и нажмите Enter. Если строка приглашения показывает «>>», нажмите Enter ещё раз. На запрос «Tailscale API access token» вставьте ключ из пункта 1. Мне ключ не присылайте.
3. Напишите мне последнюю строку вывода: «expired» или строку, которая начинается с «NOT-EXPIRED».
4. После этого отзовите ключ доступа на той же странице (Revoke).
```

Команды для пользователя. Он выполняет их сам, в своём окне; ключ доступа никуда не записывается. Блок в фигурных скобках вставляется целиком, поэтому запрос ключа появляется после вставки, а не посреди неё:

```powershell
& {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $token = Read-Host 'Tailscale API access token'
    $headers = @{ Authorization = "Bearer $token" }
    $name = (Get-Content -LiteralPath C:\code\router\spike\run\phase-b-v2.json -Raw | ConvertFrom-Json).endpoints[0].hostname
    $found = @()
    $found = @((Invoke-RestMethod -Headers $headers -Uri 'https://api.tailscale.com/api/v2/tailnet/-/devices').devices | Where-Object { $_.hostname -eq $name })
    if ($found.Count -ne 1) { "NOT-EXPIRED: devices with this name: $($found.Count)" }
    else {
        try {
            Invoke-RestMethod -Method Post -Headers $headers -Uri ("https://api.tailscale.com/api/v2/device/{0}/expire" -f $found[0].id) -ErrorAction Stop | Out-Null
            'expired'
        } catch { "NOT-EXPIRED: $($_.Exception.Message)" }
    }
}
```

Ожидается последняя строка `expired`. Продолжать только в этом случае. Строка `NOT-EXPIRED: ...` означает, что ключ не прекращён: шаг 4 не выполнять. `NOT-EXPIRED: devices with this name: 0` после красного сообщения об ошибке — неверный ключ доступа; без сообщения об ошибке — устройства с таким именем нет; число больше одного — в сети несколько устройств с этим именем, и какое из них встроенный узел, команды решить не могут. Устраните причину и повторите команды либо прекратите задачу: шаг 7, в отчёт — «(з) не проверялась». Если пользователь не хочет или не может создать ключ доступа к API, задача прекращается так же.

- [ ] **Step 4: Дождаться, пока ядро сообщит об истечении**

Сразу после ответа пользователя. Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\check_relogin.py wait-expired
```

Ожидается один из двух исходов. Ссылка без перезапуска не появилась:

```text
  State=Starting
  State=NeedsLogin
PASS Z1-expiry-reported states seen through the API service: ['Starting', 'NeedsLogin']
INFO Z2-link-without-restart 30 s after the expiry, without restarting the core: link from the API service: False, link in the log: False
```

Ссылка появилась сама: во второй строке результата `link from the API service: True`, и последней идёт строка `LOGIN-LINK <link>`. Тогда шаг 5 пропускается.

Критерий `Z1-expiry-reported`: за 9 минут служба API показала состояние `NeedsLogin` или ссылку для входа. `FAIL` означает, что ядро не узнало об истечении: в отчёт записывается, что план 1 не может определять истечение входа по службе API без перезапуска ядра; задача продолжается с шага 5.

- [ ] **Step 5: Получить ссылку перезапуском ядра**

Остановить ядро из шага 2 (оно без прав администратора):

```powershell
Stop-Process -Name sing-box -Force; Start-Sleep -Seconds 2; if (Get-Process -Name sing-box -ErrorAction SilentlyContinue) { 'STILL-RUNNING' } else { 'STOPPED' }
```

Ожидается: `STOPPED`. Запустить его снова той же командой, что в шаге 2 (инструмент PowerShell с `run_in_background: true`):

```powershell
Remove-Item -LiteralPath C:\code\router\spike\plan0\out\setup.log -ErrorAction SilentlyContinue; & 'C:\code\router\spike\dl\sing-box-1.14.4\sing-box-1.14.4-windows-amd64\sing-box.exe' run -c C:\code\router\spike\plan0\out\p0-setup.json -D C:\code\router\spike\run
```

Затем:

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\check_relogin.py wait-link restart
```

Ожидается:

```text
PASS Z3-link-after-restart after restart: link from the API service: True, link in the log: True, State=NeedsLogin
LOGIN-LINK <link>
```

Если `Z3-link-after-restart` — `FAIL`, последнее средство — выход через службу API. После него в сети появится новое устройство, а прежнее останется в консоли с истёкшим ключом; пользователь удаляет прежнее сам.

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\check_relogin.py logout; & C:\Python314\python.exe C:\code\router\spike\plan0\check_relogin.py wait-link logout
```

Ожидается `LOGOUT exit code 0`, `PASS Z3-link-after-logout ...` и строка `LOGIN-LINK`. Если ссылки нет и здесь, задача прекращается: шаг 7, в отчёт — «новая ссылка не получена ни одним способом», вопрос владельцу продукта.

- [ ] **Step 6: Войти по ссылке (нужен пользователь)**

Передайте пользователю ссылку из строки `LOGIN-LINK` в чате и попросите открыть её в браузере и войти под своей учётной записью Tailscale. Ссылка не записывается ни в план, ни в результаты. Сразу после этого, инструмент PowerShell с тайм-аутом 600000 мс:

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\check_relogin.py wait-login
```

Ожидается:

```text
  State=NeedsLogin
  State=Starting
PASS Z5-relogin login through the link in setup mode with the system exit node on: True; states: ['NeedsLogin', 'Starting']; exit nodes listed afterwards: 1
```

Критерий `Z5-relogin`: за 9 минут состояние стало `Starting` или `Running`, ссылки для входа больше нет. Это же закрывает вторую половину (д): вход в режиме настройки при включённом системном exit node завершается.

Перед тем как принять `FAIL Z5-relogin`, спросите пользователя, показала ли страница входа успех. Если он не успел войти, повторите команду `wait-login` (ссылка та же, пока ядро не перезапускалось); новая запись заменит прежнюю. `FAIL` после подтверждённого входа в браузере — провал второй половины (д).

- [ ] **Step 7: Остановить ядро и записать вывод**

```powershell
Stop-Process -Name sing-box -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 2; if (Get-Process -Name sing-box -ErrorAction SilentlyContinue) { 'STILL-RUNNING' } else { 'STOPPED' }
```

Ожидается: `STOPPED`.

Что это значит для плана 1:

- `Z1` — `PASS`: истечение входа определяется по состоянию `NeedsLogin` из службы API (поле `tailscale.backend` = `expired`).
- `Z2` с `True`: ссылку можно взять у работающего ядра. `Z2` с `False` и `Z3-link-after-restart` — `PASS`: ссылку даёт только перезапуск ядра в режиме настройки, как и описано в разделе 7; ничего менять не нужно.
- `Z3-link-after-restart` — `FAIL`, `Z3-link-after-logout` — `PASS`: `tailscale.login` при истёкшем входе должен выполнять выход через службу API; это изменение раздела 7, вопрос владельцу продукта (`report.py` печатает `Z3-link-after-restart` в строке `OWNER-QUESTION`; то же, если ссылки нет и после выхода).
- `Z5-relogin` — `FAIL`: вход по ссылке в режиме настройки при включённом системном exit node не завершается. Запасного способа в спецификации нет: вопрос владельцу продукта (`report.py` печатает запись в строке `OWNER-QUESTION`). Встроенный узел остаётся без входа, задача 5 не выполняется.

---

### Task 5: (и) Сон и смена сети — нужен пользователь

Задача требует, чтобы пользователь усыпил ПК и вынул сетевой кабель. Связь исполнителя на это время пропадает; это ожидаемо. Провал или пропуск задачи не мешает плану 1: по разделу 10 спецификации служебный процесс после пробуждения и смены сети ждёт до 60 секунд и, если проверки не проходят, перезапускает ядро. Задача отвечает на вопрос, возвращается ли дорожка VPN без перезапуска и остаётся ли соединение с exit node прямым (известная проблема ядра № 4686).

Обычный watchdog откатывается после трёх неудачных проверок связи подряд, то есть примерно через 30 секунд без VPN, и наблюдать было бы нечего. Поэтому сеанс запускается с `--max-fails 9`: откат наступает после девяти неудач подряд, это от полутора до четырёх минут. Всё это время программы дорожки VPN, включая исполнителя, остаются без связи, но напрямую не идут; при ответе с чужого адреса watchdog откатывается сразу, как и раньше.

Правила на всю задачу:

- Задачу целиком выполняет исполнитель, который сам ведёт разговор с пользователем; подчинённому агенту она не передаётся.
- Все команды задачи запускаются инструментом PowerShell с тайм-аутом 600000 мс. Пока сеанс в состоянии `ready`, каждая команда начинается с `. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat;`.
- Вопрос пользователю при живом сеансе задаётся только после `Set-P0Heartbeat`. Если ответа нет дольше 3 минут, либо ответ пришёл, а с последней команды прошло больше 3 минут, — не продолжайте опыт: выполните шаг 6 и начните новый сеанс с шага 2. Если ответ так и не пришёл, watchdog сам вернёт системный VPN не позже чем через 400 секунд после последней команды; это безопасно, но стоит ещё одного запроса UAC.
- Основная дорожка в этой задаче — только VPN (конфиг `p0-main-vpn.json`, режим не переключается), поэтому согласие на прямую основную дорожку здесь не нужно.

**Files:**
- Create: `C:\code\router\spike\plan0\observe.py`

**Interfaces:**
- Consumes: из задачи 1 — `lib.ps1` (`Set-P0Heartbeat`, `Start-P0Watchdog -MaxFails`, `Send-P0Command`, `Get-P0Session`, `Close-P0StaleSession`), `p0lib.py` (`require_state`, `session_state`, `egress_probe`, `ts_path`, `heartbeat`, `record`), `prepare.py`, конфиг `spike\run\p0-main-vpn.json`. Записи этой задачи делает `record()`, а не `record_live()`: откат сеанса после сна или смены сети здесь — ожидаемый исход опыта.
- Produces: записи `I-sleep`, `I-cable`, при наличии Wi-Fi `I-wifi` (возвращается ли дорожка VPN без перезапуска ядра) и к каждой из них запись `I-<метка>-direct` (возвращается ли прямое соединение встроенного узла с exit node — вопрос пункта (и) раздела 12); файлы `spike\plan0\out\observe-<метка>.jsonl`.

- [ ] **Step 1: Записать `observe.py`**

Файл `C:\code\router\spike\plan0\observe.py`, целиком:

```python
"""Plan 0, check (i): the embedded node across sleep/resume or a network change, without restarting the core.

usage: observe.py <label> [seconds]   label: sleep | cable | wifi; default 300 s
Samples both lanes every 2 s through the probe inbounds, keeps the watchdog heartbeat alive, and
ends early when the watchdog leaves the state "ready". The watchdog must have been started with
--max-fails 9, otherwise it rolls back after three failed health checks (about 30 s without VPN).
Two records: I-<label> - does the VPN lane come back without a core restart; I-<label>-direct - does
the direct connection of the embedded node to the exit node come back (core issue 4686).
"""
from p0lib import *


def main():
    label = sys.argv[1] if len(sys.argv) > 1 else "event"
    seconds = int(sys.argv[2]) if len(sys.argv) > 2 else 300
    status = require_state("ready")
    v = local_values()
    pid = status.get("pid")
    path_before = ts_path(v["exit_node"])
    print("OBSERVING %s for %d s, path before: %s" % (label, seconds, path_before))
    sys.stdout.flush()
    samples = []
    state = "ready"
    start = time.time()
    log_path = os.path.join(OUT, "observe-%s.jsonl" % label)
    with open(log_path, "w", encoding="ascii") as log:
        while time.time() - start < seconds:
            heartbeat()
            state, status = session_state()
            sample = {"t": round(time.time(), 1), "state": state,
                      "vpn": egress_probe(PROBE_VPN, timeout=4) == v["vpn_ip"],
                      "direct": egress_probe(PROBE_DIRECT, timeout=4) is not None}
            samples.append(sample)
            log.write(json.dumps(sample) + "\n")
            log.flush()
            if state != "ready":
                break
            time.sleep(2)
    # analysis: a wall-clock jump is the sleep. The lag is the longest stretch, anywhere in the run, with the
    # direct lane answering (the network is there) and the VPN lane not: one slow probe before the event
    # must not stand for the event itself.
    gaps = [round(b["t"] - a["t"]) for a, b in zip(samples, samples[1:]) if b["t"] - a["t"] > 15]
    failures = sum(not s["vpn"] for s in samples)
    lag, since, prev = 0, None, None
    for s in samples:
        if prev is not None and s["t"] - prev["t"] > 15:
            since = None  # after a sleep the stretch is counted from the first sample taken after it
        if since is not None:
            lag = max(lag, round(s["t"] - since))
        if s["vpn"]:
            since = None
        elif s["direct"] and since is None:
            since = s["t"]
        prev = s
    back = bool(samples) and samples[-1]["vpn"]
    same_core = state == "ready" and (read_json(STATUS) or {}).get("pid") == pid
    path_after = ts_path(v["exit_node"]) if same_core else "none"
    heartbeat()
    # Why the watchdog rolled back, if it did. The main lane here is VPN, so "traffic left through" is not
    # an owner miss: a request of a VPN lane program went past the adapter.
    reason = re.sub(r"\d{1,3}(\.\d{1,3}){3}", "<ip>", str((read_json(STATUS) or {}).get("reason") or "-"))
    leak = "traffic left through" in reason
    note = ("samples: %d, sleep gaps (s): %s, VPN probe failures: %d, longest stretch with the direct lane "
            "answering and the VPN lane not: %d s, VPN lane answering at the end: %s, watchdog state at the "
            "end: %s (reason: %s), same core process: %s, path before/after: %s/%s" % (
                len(samples), gaps, failures, lag, back, state, reason, same_core, path_before, path_after))
    happened = bool(gaps) or failures > 0   # otherwise nothing happened while observing
    verdict = bool(same_core and back and lag <= 60) if happened else None
    record("I-%s" % label, verdict, note, gaps=gaps, lag=lag, state=state, same_core=same_core, leak=leak,
           path_before=path_before, path_after=path_after)
    # the question of spec section 12 (i): does the DIRECT connection to the exit node come back
    if happened and path_before == "direct":
        record("I-%s-direct" % label, bool(same_core and path_after == "direct"),
               "direct connection to the exit node before the event; after it, without a core restart: %s"
               % path_after, path_before=path_before, path_after=path_after)
    elif happened:
        record("I-%s-direct" % label, None, "the connection to the exit node was %s before the event: whether "
               "a direct connection comes back could not be observed" % path_before,
               path_before=path_before, path_after=path_after)


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Включить деление (нужен пользователь: готовность, UAC)**

Сначала убедитесь, что встроенный узел не остался без входа. Если в задаче 4 ключ узла был прекращён или узел оказался без входа, задача 5 начинается только после `PASS Z5-relogin`. Проверка читает только `results.json` и ничего не запускает:

```powershell
& C:\Python314\python.exe -c "import json, os; p = r'C:\code\router\spike\plan0\out\results.json'; r = json.load(open(p, encoding='utf-8')) if os.path.exists(p) else {}; ok = lambda k: (r.get(k) or {}).get('ok'); lost = 'Z1-expiry-reported' in r or 'Z3-link-after-restart' in r or 'Z3-link-after-logout' in r or ok('Z0-baseline') is False; print('NODE-OK' if (ok('Z5-relogin') is True or not lost) else 'NODE-NOT-LOGGED-IN')"
```

Ожидается `NODE-OK`: задача 4 не выполнялась, либо узел в ней вход не терял, либо последняя запись `Z5-relogin` — `PASS`. При `NODE-NOT-LOGGED-IN` запрос UAC не показывать: узел без входа, включение на 45 секунд оставило бы все программы без связи и закончилось бы откатом. Вернитесь к задаче 4, шаг 2: скрипт сам покажет, с какого шага продолжать.

Перед шагом спросите пользователя, готов ли он в ближайшие 20 минут усыпить ПК и вынуть сетевой кабель, и покажите ему заранее тексты из шагов 3, 4 и 5. Запрос UAC — только после ответа «готов». Затем предупредите: сейчас появится запрос UAC от Python, его нужно принять в течение 120 секунд; отмена ничего не меняет.

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Start-P0Watchdog -Mode engage -Config p0-main-vpn.json -MaxFails 9
```

Ожидается `P0-RESULT: state=ready run=<id> ... config=p0-main-vpn.json`. Этот же запуск подтверждает, что после задачи 4 встроенный узел в сети: watchdog выключает системный exit node только после доказательства туннеля. Итог `state=rolled_back` с причиной `embedded tunnel not usable` означает, что узел без входа: вернитесь к задаче 4.

- [ ] **Step 3: Сон и пробуждение (нужен пользователь)**

Сначала напишите пользователю, затем сразу запустите наблюдение. Текст для пользователя:

```text
Сейчас я запущу наблюдение на 7 минут, и моя связь на время сна пропадёт — так и должно быть.
1. Подождите 30 секунд после этого сообщения.
2. Усыпите компьютер: Пуск → Питание → Спящий режим.
3. Подождите 60–90 секунд и разбудите его, войдите в Windows.
4. Три минуты ничего не трогайте.
Не держите компьютер во сне дольше 3 минут: сработает защита и вернёт обычный VPN, опыт придётся повторить.
Нужен именно «Спящий режим», не «Гибернация» и не «Завершение работы».
Если после пробуждения интернет не появился за 10 минут или компьютер перезагрузился — выберите прежний exit node в значке Tailscale, затем снимите sing-box.exe в Диспетчере задач, если он там есть, и напишите мне.
```

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\observe.py sleep 420
```

Скрипт делает две записи: `I-sleep` — вернулась ли дорожка VPN без перезапуска ядра, `I-sleep-direct` — вернулось ли прямое соединение встроенного узла с exit node. Три возможных исхода.

Дорожка VPN вернулась без перезапуска ядра, соединение с exit node осталось прямым:

```text
OBSERVING sleep for 420 s, path before: direct
PASS I-sleep samples: 96, sleep gaps (s): [84], VPN probe failures: 3, longest stretch with the direct lane answering and the VPN lane not: 13 s, VPN lane answering at the end: True, watchdog state at the end: ready (reason: -), same core process: True, path before/after: direct/direct
PASS I-sleep-direct direct connection to the exit node before the event; after it, without a core restart: direct
```

Дорожка VPN не вернулась, watchdog откатился и вернул системный VPN:

```text
OBSERVING sleep for 420 s, path before: direct
FAIL I-sleep samples: 61, sleep gaps (s): [84], VPN probe failures: 38, longest stretch with the direct lane answering and the VPN lane not: 139 s, VPN lane answering at the end: False, watchdog state at the end: rolled_back (reason: health failed 9 times: {'ip': None, 'anthropic_http': None, 'dns': False, 'leak': False}), same core process: False, path before/after: direct/none
FAIL I-sleep-direct direct connection to the exit node before the event; after it, without a core restart: none
```

Компьютер не засыпал (`sleep gaps (s): []`, `VPN probe failures: 0`) — запись `INFO I-sleep`, записи `I-sleep-direct` нет; договоритесь с пользователем и повторите шаг. То же при `sleep gaps (s): []` и `watchdog state at the end: ready` с любым первым словом строки: компьютер не засыпал, а одна из проверок просто осталась без ответа; при повторе новые записи заменят прежние.

Критерий `PASS I-sleep`: после события ядро то же (тот же процесс, watchdog в состоянии `ready`), в конце наблюдения дорожка VPN отвечает, и самый долгий отрезок, когда прямая дорожка уже отвечала, а дорожка VPN ещё нет, не длиннее 60 секунд (значение `longest stretch`). Отрезок ищется по всему наблюдению, а не от первой неудачной проверки; после сна он считается с первого измерения после пробуждения. Значение считается по моментам измерений, его точность — около 10 секунд.

Запись `I-sleep-direct` отвечает на вопрос пункта (и) раздела 12. `PASS`: соединение с exit node было прямым до события и осталось прямым после него без перезапуска ядра. `FAIL`: после события оно идёт через ретранслятор (`direct/relay`, проблема ядра № 4686) либо ядро перезапускалось (`direct/none`). `INFO`: до события соединение не было прямым, наблюдать нечего.

После шага соберите строки watchdog; адреса в выводе закрыты:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Get-Content C:\code\router\spike\run\watchdog.log -Tail 25 | ForEach-Object { $_ -replace '\b\d{1,3}(\.\d{1,3}){3}\b', '<ip>' }; Get-P0Session
```

Ожидается: строки `health fail N: ...` за время события и либо `P0-SESSION: state=ready ...`, либо строки `ROLLBACK: health failed 9 times` и `P0-SESSION: state=rolled_back ... system_vpn_ok=True`.

Строка `ROLLBACK: traffic left through <ip> instead of the exit node` — другой исход. Основная дорожка здесь VPN, поэтому это не промах с владельцем соединения: запрос программы дорожки VPN ушёл мимо адаптера. Это не «дорожка VPN не вернулась», а уход трафика в обход VPN после события (критерий успеха 3 из раздела 1, строка «Сон и пробуждение, смена сети» раздела 10). Скрипт видит эту причину сам: в тексте записи стоит `reason: traffic left through <ip> ...`, `report.py` печатает запись в строке `OWNER-QUESTION` и добавляет её в абзац вопросов владельцу продукта приложения A.

Если сеанс откатился, шаги 4 и 5 требуют нового запуска (шаг 2, нужен пользователь: UAC). Спросите пользователя, продолжать ли.

- [ ] **Step 4: Отключение и подключение кабеля (нужен пользователь)**

Только при `P0-SESSION: state=ready`. Текст для пользователя:

```text
Сейчас я запущу наблюдение на 5 минут.
1. Подождите 30 секунд после этого сообщения.
2. Выньте сетевой кабель из компьютера на 15 секунд и вставьте обратно.
3. Три минуты ничего не трогайте.
Если после возврата кабеля интернет не появился за 10 минут или компьютер перезагрузился — выберите прежний exit node в значке Tailscale, затем снимите sing-box.exe в Диспетчере задач, если он там есть, и напишите мне.
```

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\observe.py cable 300
```

Ожидается строка `PASS I-cable ...`, `FAIL I-cable ...` или `INFO I-cable ...` того же вида, что в шаге 3, но с `sleep gaps (s): []`, и за ней строка `I-cable-direct` (кроме случая `INFO I-cable`). Критерии те же: для `I-cable` — то же ядро, дорожка VPN отвечает в конце, значение `longest stretch` не больше 60 секунд; для `I-cable-direct` — соединение с exit node осталось прямым. Если в строке `VPN probe failures: 0`, кабель не вынимали: договоритесь с пользователем и повторите шаг.

Если watchdog откатился, пока кабель был вынут, итог может быть `state=restore_unverified`: системный exit node включён обратно, но связь проверить было нечем. После подключения кабеля выполните проверку из шага 6.

- [ ] **Step 5: Смена сети кабель ↔ Wi-Fi (нужен пользователь; только если есть Wi-Fi)**

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Get-NetAdapter -Physical | Select-Object Name, InterfaceDescription, Status | Format-Table -AutoSize
```

Если в списке нет адаптера Wi-Fi, шаг не выполняется; в приложении A смена сети Wi-Fi ↔ кабель остаётся в абзаце «Не проверялось»: опыт с кабелем не меняет сетевой интерфейс, а проблема ядра № 4686 проявляется именно при его смене. Если адаптер есть и сеанс в состоянии `ready`, текст для пользователя:

```text
Сейчас я запущу наблюдение на 5 минут.
1. Подключите Wi-Fi к любой доступной сети, кабель пока не трогайте.
2. Через 30 секунд после этого сообщения выньте сетевой кабель на 60 секунд: компьютер перейдёт на Wi-Fi.
3. Вставьте кабель обратно и три минуты ничего не трогайте.
Если после возврата кабеля интернет не появился за 10 минут или компьютер перезагрузился — выберите прежний exit node в значке Tailscale, затем снимите sing-box.exe в Диспетчере задач, если он там есть, и напишите мне.
```

Инструмент PowerShell, тайм-аут 600000 мс:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; & C:\Python314\python.exe C:\code\router\spike\plan0\observe.py wifi 300
```

Ожидается строка `PASS I-wifi ...`, `FAIL I-wifi ...` или `INFO I-wifi ...` и за ней строка `I-wifi-direct` (кроме случая `INFO I-wifi`); критерии те же, что в шаге 4. Именно `I-wifi-direct` отвечает на вопрос о проблеме ядра № 4686.

- [ ] **Step 6: Закончить сеанс и проверить систему**

Инструмент PowerShell, тайм-аут 600000 мс. Если сеанс уже откатился, первая команда напечатает `P0-RESULT: NO-LIVE-SESSION`; это нормально.

```powershell
. C:\code\router\spike\plan0\lib.ps1; Set-P0Heartbeat; Send-P0Command -Action stop; & C:\Python314\python.exe C:\code\router\spike\plan0\prepare.py
```

Ожидается `P0-RESULT: state=rolled_back ... system_vpn_ok=True` (или `NO-LIVE-SESSION`) и `PREPARE-OK` в конце. При строке `WRONG` покажите пользователю ручное восстановление из Global Constraints.

Если ПК во время задачи перезагружался, выключался или не проснулся, watchdog и ядро не существуют, а системный exit node остался выключенным: эта настройка переживает перезагрузку (спецификация, 5.7). Тогда первая команда печатает `P0-RESULT: NO-LIVE-SESSION - stale status ...`, а `prepare.py` — `STATE no live watchdog session WRONG`: это устаревший `status.json`. Порядок действий:

1. Убедитесь, что в выводе `prepare.py` строки `STATE no sing-box.exe running` и `STATE system exit node on` — `OK`. Если вторая — `WRONG`, пользователь ещё не выбрал exit node: покажите ему ручное восстановление из Global Constraints и повторите команду шага.
2. Закройте устаревшее состояние и проверьте систему ещё раз:

```powershell
. C:\code\router\spike\plan0\lib.ps1; Close-P0StaleSession; & C:\Python314\python.exe C:\code\router\spike\plan0\prepare.py
```

Ожидается `P0-RESULT: state=restore_unverified ... reason=stale status closed, the watchdog process is gone (was ready) system_vpn_ok=False` и `PREPARE-OK` в конце. Запись `I-...` прерванного опыта не сделана; в отчёт записывается, что опыт прерван перезагрузкой. Повтор опыта — новый сеанс с шага 2, с согласия пользователя.

Что это значит для плана 1:

- `PASS` в `I-sleep` и `I-cable`: ожидание до 60 секунд из раздела 10 достаточно, перезапуск ядра после пробуждения и смены сети обычно не понадобится.
- `FAIL` в `I-<метка>` с причиной `health failed 9 times` либо без отката (ядро то же, но дорожка VPN не вернулась за 60 секунд): срабатывает перезапуск ядра, уже описанный в разделе 10; менять спецификацию не нужно.
- `FAIL` в `I-<метка>` с причиной `traffic left through`: запрос программы дорожки VPN ушёл мимо VPN после события. Это нарушение критерия успеха 3, а не «дорожка VPN не вернулась»: вопрос владельцу продукта, `report.py` печатает запись в строке `OWNER-QUESTION`.
- `I-<метка>-direct` — `FAIL` при `PASS I-<метка>`: прямое соединение без перезапуска ядра не вернулось (проблема ядра № 4686), дорожка VPN работает через ретранслятор. План 1 показывает это полем `tailscale.path`; вопрос, перезапускать ли ядро ради прямого соединения, сообщается владельцу продукта в шаге 7 задачи 6 и план 1 не задерживает. При `FAIL I-<метка>` запись `I-<метка>-direct` только повторяет его исход: ядро перезапускалось.
- `I-<метка>-direct` — `INFO`: соединение с exit node и до события шло через ретранслятор, ответа на вопрос пункта (и) нет; в приложении A строка остаётся со словом «наблюдение».

---

### Task 6: Результаты в приложение A, три решения, коммит

Задача выполняется всегда: и после полного прохождения, и после остановки на любом шаге. Невыполненные проверки попадут в абзац «Не проверялось».

**Files:**
- Create: `C:\code\router\spike\plan0\report.py`
- Modify: `C:\code\router\docs\superpowers\specs\2026-11-13-splitlane-stage1-design.md` (приложение A: новый блок «Проверки плана 0» и абзац «Не проверялось»)

**Interfaces:**
- Consumes: `spike\plan0\out\results.json` (записи задач 2–5), `p0lib.py` (`read_json`, `write_atomic`, `local_values`, `MODES`, `OUT`, `RESULTS`, `LANES`, `TS`).
- Produces: `spike\plan0\out\appendix-a.md`; правка спецификации; коммит.

- [ ] **Step 1: Записать `report.py`**

Файл `C:\code\router\spike\plan0\report.py`, целиком:

```python
"""Plan 0, final task: results table, the three decisions for Plan 1, text for Appendix A, privacy scan.

usage: report.py                  print the table and the decisions (ASCII), write out/appendix-a.md (Russian, UTF-8)
       report.py apply <spec.md>  put the block into Appendix A of the spec (idempotent)
       report.py scan <file>...   look for local addresses, names and secrets in files that go to the repository
"""
import subprocess

from p0lib import *

WORD = {True: "подтверждено", False: "НЕ подтверждено", None: "наблюдение", "missing": "не проверялось"}

# check id -> (spec check letter, Russian description)
LABELS = [
    ("A0-mode-list", "а", "ядро знает ровно пять режимов из правил, режим по умолчанию читается через `GET /configs`"),
    ("A1-start-vpn", "а", "запуск ядра в режиме `vpn` (`default_mode`): маршруты и DNS по проекту"),
    ("A1-start-vpn-localdns", "а", "запуск ядра в режиме `vpn-localdns`"),
    ("A1-start-direct", "а", "запуск ядра в режиме `direct`"),
    ("A1-start-direct-localdns", "а", "запуск ядра в режиме `direct-localdns`"),
    ("A1-start-alldirect", "а", "запуск ядра в режиме `alldirect`"),
    ("A2-transitions-all-pairs", "а", "все 20 переходов между пятью режимами через `PATCH /configs` с проверкой "
                                      "чтением, маршрутом незакреплённой программы и сервером DNS"),
    ("A2-transitions-from-vpn-localdns", "а", "переходы из стартового режима `vpn-localdns` в каждый и обратно"),
    ("A2-transitions-from-direct", "а", "переходы из стартового режима `direct` в каждый и обратно"),
    ("A2-transitions-from-direct-localdns", "а", "переходы из стартового режима `direct-localdns` в каждый и обратно"),
    ("A2-transitions-from-alldirect", "а", "переходы из стартового режима `alldirect` в каждый и обратно"),
    ("A3-unknown-mode", "а", "переход на неизвестный режим: API отвечает успехом, режим не меняется"),
    ("A5-open-connections", "а", "смена режима не закрывает открытые соединения; закрытие по идентификатору "
                                 "через Clash API работает"),
    ("B0-setup-api", "б", "служба API отвечает в режиме настройки"),
    ("D2-setup-exit-nodes", "б", "служба API в режиме настройки отдаёт список exit node; список и признаки "
                                 "«в сети» совпадают с данными системного клиента"),
    ("B5-setup-api-link", "б", "служба API в режиме настройки отдаёт ссылку для входа (та же, что в журнале)"),
    ("B1-setup-secret", "б", "служба API отклоняет запрос с неверным секретом"),
    ("B2-hold-api", "б", "служба API при поднятом адаптере до переключения системного exit node: состояние, "
                         "выбранный exit node, вид соединения"),
    ("B3-normal-api", "б", "служба API в обычном режиме: состояние, список exit node с признаками «в сети» как "
                           "у системного клиента, выбранный узел"),
    ("B4-normal-api-extras", "б", "служба API в обычном режиме, дополнения: вид соединения с exit node, режим ядра"),
    ("V1-space-folder", "в", "правило папки, путь с пробелами, выражение в другом регистре"),
    ("V2-brackets-file", "в", "правило файла, путь с круглыми и квадратными скобками"),
    ("V3-cyrillic-file", "в", "правило файла, кириллица в имени папки и файла, выражение в другом регистре"),
    ("V9-inline-regex", "в", "встроенное правило конфига с `process_path_regex` (как правила 5 и 6 из 6.2)"),
    ("V5-vpn-file-direct-main", "в", "файл правил дорожки VPN срабатывает при основной дорожке «Напрямую»"),
    ("V6-hot-reload", "в", "перечитывание файла после атомарной замены, не дольше 3 секунд"),
    ("V7-system-path", "в", "псевдопуть `:System` (соединение ядра Windows)"),
    ("V4-nested-invert", "в", "вложенные правила разных дорожек: логическое правило с `invert`"),
    ("V8-broken-file", "в", "испорченный файл правил: строка `reload rule-set` в журнале, прежние правила действуют"),
    ("G2-hold-probe-direct", "г", "вход `probe-direct` при включённом системном exit node (адаптер без перехвата)"),
    ("G3-engage-probe-direct", "г", "вход `probe-direct` перед переключением системного exit node (полный адаптер, "
                                    "шаг 5 включения)"),
    ("D1-setup-state", "д", "режим настройки при включённом системном exit node: узел с сохранённым входом "
                            "получает состояние"),
    ("D2-setup-exit-nodes", "д", "режим настройки: список exit node доступен"),
    ("D5-setup-login-link", "д", "режим настройки, пустое состояние узла: ядро получает ссылку для входа"),
    ("Z5-relogin", "д", "режим настройки: вход по ссылке завершается"),
    ("Z1-expiry-reported", "з", "истечение ключа узла видно через службу API (состояние `NeedsLogin`)"),
    ("Z2-link-without-restart", "з", "появляется ли новая ссылка без перезапуска ядра"),
    ("Z3-link-after-restart", "з", "новая ссылка появляется после перезапуска ядра в режиме настройки"),
    ("Z3-link-after-logout", "з", "новая ссылка появляется после выхода через службу API"),
    ("I-sleep", "и", "сон и пробуждение: дорожка VPN возвращается без перезапуска ядра"),
    ("I-cable", "и", "отключение и подключение кабеля: дорожка VPN возвращается без перезапуска ядра"),
    ("I-wifi", "и", "смена сети кабель ↔ Wi-Fi: дорожка VPN возвращается без перезапуска ядра"),
    ("I-sleep-direct", "и", "сон и пробуждение: прямое соединение встроенного узла с exit node возвращается "
                            "без перезапуска ядра"),
    ("I-cable-direct", "и", "отключение и подключение кабеля: прямое соединение встроенного узла с exit node "
                            "возвращается без перезапуска ядра"),
    ("I-wifi-direct", "и", "смена сети кабель ↔ Wi-Fi: прямое соединение встроенного узла с exit node "
                           "возвращается без перезапуска ядра (проблема ядра № 4686)"),
    ("K1-host-names", "к", "имена узлов в списке соединений Clash API при `dns.reverse_mapping`"),
    ("K5-host-alias", "к", "какое имя показывается для адреса, полученного через цепочку CNAME"),
    ("K2-tcp-lanes", "к", "TCP по дорожкам при включённом обратном сопоставлении"),
    ("K3-udp-vpn-sizes", "к", "UDP по дорожке VPN, пакеты до 1400 байт, без разбора протоколов"),
    ("K4-udp-direct", "к", "UDP по прямой дорожке"),
    ("K3n-udp-vpn-sizes", "к", "то же UDP по дорожке VPN с выключенным `reverse_mapping` (контроль)"),
]
# a FAIL here stops the work (spec section 12): (v) by process_path_regex and (g) at step 5 of the engage order
STOP = ["V1-space-folder", "V2-brackets-file", "V3-cyrillic-file", "V9-inline-regex", "V5-vpn-file-direct-main",
        "V6-hot-reload", "V7-system-path", "G3-engage-probe-direct"]
# a FAIL here has no agreed fallback in the spec and is a question to the product owner: setup mode itself (d),
# including the login that must complete in it, and the test-only pin of the executor's programs
OWNER = ["D1-setup-state", "D5-setup-login-link", "Z5-relogin", "A-pin"]
# (checks, text): the text stays in the "not tested" paragraph until one of the checks has a PASS or FAIL.
# Re-plugging the same cable does not change the network interface, so only the Wi-Fi run covers a network change.
NOT_TESTED = [
    (["I-sleep"], "сон и пробуждение"),
    (["I-wifi"], "смена сети Wi-Fi ↔ кабель"),
    ([], "трафик WSL"),
    ([], "длительная работа"),
    (["B3-normal-api"], "служба API ядра"),
    (["A2-transitions-all-pairs"], "правила с `clash_mode`"),
    (["V1-space-folder"], "правила `process_path_regex`"),
    (["V5-vpn-file-direct-main"], "срабатывание файла правил дорожки VPN при основной дорожке «Напрямую»"),
    (["K1-host-names"], "имена узлов в соединениях (`dns.reverse_mapping`)"),
    (["G3-engage-probe-direct"], "вход `probe-direct` при включённом системном exit node"),
    (["D1-setup-state"], "режим настройки"),
    (["Z1-expiry-reported", "Z3-link-after-restart"], "повторный вход после истечения срока"),
    ([], "запуск через планировщик задач"),
    ([], "завершение сеанса Windows"),
    ([], "время возврата системного exit node при падении ядра"),
]


def verdict(results, check):
    return results[check]["ok"] if check in results else "missing"


def dns_only(results, check):
    """A FAIL whose only cause is a DNS server that was chosen correctly but did not answer (spec 6.4, not 6.3)."""
    return verdict(results, check) is False and bool(results[check].get("data", {}).get("dns_only"))


def decide(results, checks):
    """primary when every check passed; fallback when one failed; open when one was not run.

    A FAIL that is only about an unanswered DNS server does not count against the mode mechanism:
    the mode was entered and the right server was chosen. It goes to the owner questions instead.
    """
    states = [True if dns_only(results, c) else verdict(results, c) for c in checks]
    if any(s is False for s in states):
        return "fallback", [c for c, s in zip(checks, states) if s is False]
    if any(s == "missing" for s in states):
        return "open", [c for c, s in zip(checks, states) if s == "missing"]
    return "primary", []


def owner_questions(results):
    """Failed checks that neither stop the work by section 12 nor have a fallback agreed in the spec."""
    found = [c for c in OWNER if verdict(results, c) is False]
    found += ["%s (DNS path, 6.4)" % c for c in sorted(results) if dns_only(results, c)]
    if verdict(results, "A5-open-connections") is False and results["A5-open-connections"]["data"].get("close_failed"):
        found.append("A5-open-connections (closing a connection by id, 8.3)")
    if verdict(results, "Z3-link-after-restart") is False:
        found.append("Z3-link-after-restart (a new login link only after a logout or not at all, section 7)")
    # K3 failed and the control run without reverse_mapping did not clear it: not the setting, a UDP path problem
    if verdict(results, "K3-udp-vpn-sizes") is False and verdict(results, "K3n-udp-vpn-sizes") is not True:
        found.append("K3-udp-vpn-sizes (large UDP datagrams through the VPN lane, Appendix A)")
    found += ["%s (a request of a VPN lane program left past the VPN after the event, criterion 3)" % c
              for c in ("I-sleep", "I-cable", "I-wifi") if results.get(c, {}).get("data", {}).get("leak")]
    return found


def decisions(results):
    # A3-unknown-mode is not an input: the helper sends only the five known modes and reads GET /configs
    # after every switch, so 6.3 works whatever the API answers to an unknown mode
    modes = (["A0-mode-list", "A2-transitions-all-pairs"]
             + ["A1-start-%s" % m for m in MODES] + ["A2-transitions-from-%s" % m for m in MODES if m != "vpn"])
    api = ["B0-setup-api", "D2-setup-exit-nodes", "B5-setup-api-link", "B3-normal-api"]
    return [
        ("1", "6.3, переключатели без перезапуска", decide(results, modes),
         "режим ядра (`clash_mode`), как в 6.3", "запасной способ из 6.3: отдельные перечитываемые файлы правил"),
        ("2", "раздел 7, источник данных о узле", decide(results, api),
         "служба API ядра", "запасной способ из раздела 7: журнал ядра и системный клиент"),
        ("3", "8.2, вложенные правила разных дорожек", decide(results, ["V4-nested-invert"]),
         "логическое правило с `invert` в файле правил",
         "запасной способ из 8.2: правила 9 и 10 меняются местами, вложенное правило прямой дорожки отклоняется"),
    ]


def appendix(results):
    lines = ["Проверки плана 0 (%s), та же система; адреса и имена устройств не приводятся:" % time.strftime("%Y-%m-%d"),
             ""]
    for check, letter, text in LABELS:
        state = verdict(results, check)
        if state == "missing" and check in ("K3n-udp-vpn-sizes", "Z3-link-after-logout", "I-wifi", "I-wifi-direct",
                                            "K5-host-alias"):
            continue  # optional branches that were not needed or could not be run
        extra = ""
        data = results.get(check, {}).get("data", {})
        if check == "V6-hot-reload" and data.get("seconds"):
            extra = " (секунд: %s)" % ", ".join(str(x) for x in data["seconds"])
        if check in ("B2-hold-api", "B4-normal-api-extras") and data.get("path"):
            extra = " (вид соединения: %s)" % data["path"]
        if check == "Z2-link-without-restart" and "link" in data:
            extra = " (ссылка без перезапуска %s)" % ("появилась" if data["link"] else "не появилась")
        if check == "Z1-expiry-reported" and data.get("states"):
            extra = " (состояния: %s)" % ", ".join(str(s) for s in data["states"])
        if check.startswith("A2-") and data.get("retries"):
            extra = " (повторных запросов из-за неопределённого владельца соединения: %d)" % data["retries"]
        if dns_only(results, check):
            extra = " (режим включался, нужный сервер DNS выбирался, но не отвечал: сбой пути DNS, 6.4)"
        if check.startswith("I-") and check.endswith("-direct") and data:
            extra = " (вид соединения с exit node до события и после него: %s и %s)" % (
                data.get("path_before"), data.get("path_after"))
        elif check.startswith("I-") and data:
            extra = " (самый долгий отрезок, когда сеть уже отвечала, а дорожка VPN ещё нет: %s с)" % data.get("lag")
            if data.get("leak"):
                extra = extra[:-1] + "; откат из-за запроса, ушедшего мимо VPN)"
        if check == "D1-setup-state" and data.get("state"):
            extra = " (состояние `%s`)" % data["state"]
        if check == "K5-host-alias" and "same" in data:
            extra = " (показано %s)" % ("запрошенное имя" if data["same"] else
                                        "каноническое имя из конца цепочки, а не запрошенное")
        lines.append("- (%s) %s: %s%s." % (letter, text[0].upper() + text[1:], WORD[state], extra))
    lines += ["", "Решения для плана 1 по итогам плана 0:", ""]
    for number, where, (kind, why), primary, fallback in decisions(results):
        if kind == "primary":
            lines.append("%s. %s: основной способ — %s." % (number, where[0].upper() + where[1:], primary))
        elif kind == "fallback":
            lines.append("%s. %s: %s (не подтверждено: %s)." % (number, where[0].upper() + where[1:], fallback,
                                                                ", ".join(why)))
        else:
            lines.append("%s. %s: не определено, не выполнены проверки %s." % (number, where[0].upper() + where[1:],
                                                                             ", ".join(why)))
    stop = [c for c in STOP if verdict(results, c) is False]
    if stop:
        lines += ["", "Работа остановлена, вопрос возвращён владельцу продукта (раздел 12): не подтверждено %s." %
                  ", ".join(stop)]
    owner = owner_questions(results)
    if owner:
        lines += ["", "Вопросы владельцу продукта, для которых в спецификации нет запасного способа: %s." %
                  ", ".join(owner)]
    left = [text for checks, text in NOT_TESTED if not any(verdict(results, c) in (True, False) for c in checks)]
    not_tested = "Не проверялось: %s." % ", ".join(left)
    return "\n".join(lines) + "\n", not_tested


def apply(spec_path, block, not_tested):
    with open(spec_path, "r", encoding="utf-8", newline="") as f:
        text = f.read()
    start = text.find("Проверки плана 0 (")
    end = text.find("Не проверялось:")
    if end < 0:
        die("the paragraph 'Не проверялось:' was not found in the spec")
    tail_end = text.find("\n", end)
    tail_end = len(text) if tail_end < 0 else tail_end
    head = text[:start] if 0 <= start < end else text[:end]
    text = head + block + "\n" + not_tested + text[tail_end:]
    with open(spec_path, "w", encoding="utf-8", newline="") as f:
        f.write(text)
    print("APPLIED to %s" % os.path.basename(spec_path))


def scan(paths):
    secret = {}
    v = local_values()
    for key in ("exit_node", "vpn_ip", "hostname", "clash_secret", "api_secret"):
        secret[v[key]] = key
    for key, value in (read_json(LANES) or {}).items():
        secret[value] = "lane address (%s)" % key
    for name in ("USERNAME", "COMPUTERNAME"):
        if os.environ.get(name):
            secret[os.environ[name]] = name
    try:
        p = subprocess.run([TS, "status", "--json"], capture_output=True, timeout=20, creationflags=NO_WINDOW)
        status = json.loads(p.stdout.decode("utf-8", "replace"))
        tailnet = status.get("CurrentTailnet") or {}
        for value in (tailnet.get("Name"), tailnet.get("MagicDNSSuffix")):
            secret[value] = "tailnet name"
        for peer in [status.get("Self") or {}] + list((status.get("Peer") or {}).values()):
            secret[peer.get("HostName")] = "device name"
            secret[(peer.get("DNSName") or "").rstrip(".")] = "device name"
            for ip in peer.get("TailscaleIPs") or []:
                secret[ip] = "tailnet address"
        for user in (status.get("User") or {}).values():
            secret[user.get("LoginName")] = "account name"
            secret[user.get("DisplayName")] = "account name"
    except (OSError, ValueError):
        print("PRIVACY-WARN the system Tailscale client did not answer; names of devices are not scanned")
    secret = {k.lower(): what for k, what in secret.items() if k and len(k) >= 4}
    # addresses that are part of the design or of the self-test samples (documentation ranges)
    # and the published range of the executor's API from the test-only rule 9
    allowed = {"127.0.0.1", "1.1.1.1", "0.0.0.0", "100.64.0.0", "172.19.0.0", "172.19.0.1", "172.19.0.2",
               "198.18.0.1", "192.0.2.1", "100.64.0.1", "100.64.0.2", "203.0.113.10", "160.79.104.0"}
    hits = 0
    for path in paths:
        with open(path, "r", encoding="utf-8") as f:
            for number, line in enumerate(f, 1):
                low = line.lower()
                kinds = {what for value, what in secret.items() if value in low}
                kinds |= {"address" for ip in re.findall(r"(?<![\d.])\d{1,3}(?:\.\d{1,3}){3}(?![\d.])", line)
                          if ip not in allowed}
                # the needles are assembled so that this file itself does not trip the scan
                if ("login.tailscale.com" + "/a/") in low or ("ts" + "key-") in low:
                    kinds.add("login link or key")
                if re.search(r"(?<![0-9a-f])[0-9a-f]{32}(?![0-9a-f])", low):
                    kinds.add("32 hex digits")
                for kind in sorted(kinds):
                    hits += 1
                    print("PRIVACY-HIT %s:%d %s" % (os.path.basename(path), number, kind))
    print("PRIVACY-OK" if hits == 0 else "PRIVACY-FAILED (%d hits)" % hits)
    sys.exit(0 if hits == 0 else 1)


def main():
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    if command == "scan":
        scan(sys.argv[2:])
    results = read_json(RESULTS) or {}
    block, not_tested = appendix(results)
    if command == "apply":
        apply(sys.argv[2], block, not_tested)
        return
    for check in sorted(results):
        print("%-5s %-36s %s" % ({True: "PASS", False: "FAIL", None: "INFO"}[results[check]["ok"]], check,
                                 results[check]["note"]))
    for number, where, (kind, why), _, _ in decisions(results):
        print("DECISION %s: %s %s" % (number, kind.upper(), ", ".join(why)))
    stop = [c for c in STOP if verdict(results, c) is False]
    print("STOP-AND-RETURN-TO-OWNER: %s" % (", ".join(stop) if stop else "no"))
    owner = owner_questions(results)
    print("OWNER-QUESTION: %s" % (", ".join(owner) if owner else "no"))
    write_atomic(os.path.join(OUT, "appendix-a.md"), block + "\n" + not_tested + "\n")
    print("WRITTEN out\\appendix-a.md")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Получить таблицу и решения**

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\report.py
```

Ожидается: по строке на каждую запись (`PASS`, `FAIL` или `INFO`, идентификатор, пояснение), затем три строки решений, строка об остановке и строка вопросов владельцу. При полном успехе конец вывода такой:

```text
DECISION 1: PRIMARY 
DECISION 2: PRIMARY 
DECISION 3: PRIMARY 
STOP-AND-RETURN-TO-OWNER: no
OWNER-QUESTION: no
WRITTEN out\appendix-a.md
```

Правила, по которым скрипт принимает решения:

| Решение | `PRIMARY` (основной способ), если `PASS` у всех записей | `FALLBACK` | `OPEN` |
|---|---|---|---|
| 1. 6.3, `clash_mode` | `A0-mode-list`, `A2-transitions-all-pairs`, пять `A1-start-*`, четыре `A2-transitions-from-*` | хотя бы одна из них `FAIL` | `FAIL` нет, но какая-то не выполнялась |
| 2. Раздел 7, служба API | `B0-setup-api`, `D2-setup-exit-nodes`, `B5-setup-api-link`, `B3-normal-api` | хотя бы одна `FAIL` | какая-то не выполнялась |
| 3. 8.2, логическое правило с `invert` | `V4-nested-invert` | `FAIL` | не выполнялась |

В решении 1 запись `FAIL`, единственная причина которой — `dns-no-answer` (нужный сервер DNS выбран, но не ответил), считается пройденной: режим включился и правило DNS сработало. Сама запись попадает в строку `OWNER-QUESTION`. `A3-unknown-mode` в решение 1 не входит: схема 6.3 от ответа API на неизвестный режим не зависит (шаг 10 задачи 3); `FAIL` этой записи сообщается владельцу продукта как расхождение с текстом 6.3 и план 1 не задерживает.

`STOP-AND-RETURN-TO-OWNER` перечисляет не пройденные проверки, которые по разделу 12 останавливают работу: `V1`, `V2`, `V3`, `V9`, `V5`, `V6`, `V7` (провал (в) по `process_path_regex`) и `G3` (провал (г)). `G2` в этот список не входит: это наблюдение об адаптере удержания, которого в продукте нет.

`OWNER-QUESTION` перечисляет не пройденные проверки, для которых в спецификации нет ни запасного способа, ни правила остановки: `D1-setup-state`, `D5-setup-login-link` и `Z5-relogin` (не работает сам режим настройки или вход в нём не завершается, проверка (д)), `A-pin` (не подтверждено закрепление программ исполнителя), записи с пометкой `(DNS path, 6.4)`, `A5-open-connections` с пометкой `(closing a connection by id, 8.3)`, `Z3-link-after-restart` (новая ссылка для входа не появляется после перезапуска ядра, раздел 7), `K3-udp-vpn-sizes` (запись осталась `FAIL`, и контрольный прогон `K3n` провал не снял: расхождение с приложением A), записи `I-sleep`, `I-cable`, `I-wifi` с пометкой `(a request of a VPN lane program left past the VPN after the event, criterion 3)`.

При `OPEN` нужно либо выполнить пропущенный шаг, либо сообщить владельцу продукта, что решение не принято и почему (например, пользователь не дал согласия на шаги с прямой основной дорожкой).

- [ ] **Step 3: Прочитать текст для приложения A**

Откройте `C:\code\router\spike\plan0\out\appendix-a.md` инструментом чтения файлов (файл в UTF-8). В нём блок вида:

```text
Проверки плана 0 (2026-11-13), та же система; адреса и имена устройств не приводятся:

- (а) Ядро знает ровно пять режимов из правил, режим по умолчанию читается через `GET /configs`: подтверждено.
- (а) Запуск ядра в режиме `vpn` (`default_mode`): маршруты и DNS по проекту: подтверждено.
...
- (к) UDP по прямой дорожке: подтверждено.

Решения для плана 1 по итогам плана 0:

1. 6.3, переключатели без перезапуска: основной способ — режим ядра (`clash_mode`), как в 6.3.
2. Раздел 7, источник данных о узле: основной способ — служба API ядра.
3. 8.2, вложенные правила разных дорожек: основной способ — логическое правило с `invert` в файле правил.

Не проверялось: смена сети Wi-Fi ↔ кабель, трафик WSL, длительная работа, запуск через планировщик задач, завершение сеанса Windows, время возврата системного exit node при падении ядра.
```

Пункт «смена сети Wi-Fi ↔ кабель» остаётся в этом абзаце, если шаг 5 задачи 5 не выполнялся (на ПК нет адаптера Wi-Fi): опыт с кабелем сетевой интерфейс не меняет.

Проверьте глазами: в тексте нет адресов, имён устройств и сети, ссылок для входа. Слова при записях: «подтверждено» — `PASS`, «НЕ подтверждено» — `FAIL`, «наблюдение» — `INFO`, «не проверялось» — проверка не выполнялась.

- [ ] **Step 4: Внести блок в спецификацию**

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\report.py apply C:\code\router\docs\superpowers\specs\2026-11-13-splitlane-stage1-design.md
git -C C:\code\router diff --stat
```

Ожидается `APPLIED to 2026-11-13-splitlane-stage1-design.md` и одна изменённая строка статистики для файла спецификации. Скрипт вставляет блок «Проверки плана 0» перед абзацем «Не проверялось:» в приложении A и заменяет этот абзац новым; повторный запуск заменяет блок, а не добавляет второй. Просмотрите `git -C C:\code\router diff`: изменения должны быть только в конце приложения A.

Если после задач 2–5 остались наблюдения, которых нет в записях (например, причина самопроизвольного отката в задаче 3, опыт задачи 5, прерванный перезагрузкой, или расхождение с приложением A из шага 8 задачи 3), допишите их отдельными пунктами в конец блока «Проверки плана 0» вручную, по одному предложению, без адресов и имён. Подчинённому агенту эти наблюдения передаёт основной сеанс. После ручных дополнений `report.py apply` не повторять: он заменяет блок целиком.

- [ ] **Step 5: Проверить, что личные данные не попадают в коммит**

```powershell
& C:\Python314\python.exe C:\code\router\spike\plan0\report.py scan C:\code\router\docs\superpowers\specs\2026-11-13-splitlane-stage1-design.md C:\code\router\docs\superpowers\plans\2026-11-13-splitlane-plan-0-core-checks.md
git -C C:\code\router status --short
```

Ожидается `PRIVACY-OK` и в выводе `git status` только два файла из `docs/`; строк с `spike/` нет. Скрипт ищет в файлах адрес exit node, внешний адрес VPN, прямой адрес, имена и адреса устройств сети Tailscale, имя сети, имена учётных записей, имя компьютера и пользователя Windows, оба секрета, любые адреса IPv4 кроме служебных, ссылки для входа и строки из 32 шестнадцатеричных цифр. При `PRIVACY-HIT` исправьте указанную строку и повторите шаг; коммит без `PRIVACY-OK` не создаётся.

- [ ] **Step 6: Коммит**

```powershell
git -C C:\code\router add docs/superpowers/specs/2026-11-13-splitlane-stage1-design.md docs/superpowers/plans/2026-11-13-splitlane-plan-0-core-checks.md
git -C C:\code\router commit -m "docs: record Plan 0 core check results in spec Appendix A"
git -C C:\code\router status --short
```

Ожидается: коммит создан, `git status --short` ничего не печатает.

- [ ] **Step 7: Сообщить итог владельцу продукта**

Сообщение содержит: три решения (или причину, по которой решение не принято); список записей `FAIL` с одной фразой о каждой; строки `STOP-AND-RETURN-TO-OWNER` и `OWNER-QUESTION`; что осталось в абзаце «Не проверялось»; сколько раз понадобился запрос UAC. Подчинённый агент возвращает это сообщение основному сеансу, тот передаёт его владельцу продукта.

Решение `FALLBACK` ответа владельца продукта не ждёт: запасные способы описаны и согласованы в 6.3, 7 и 8.2, интерфейс служебного процесса от выбора не зависит; план 1 пишется под записанное решение, владельцу оно сообщается. План 1 не начинается до ответа владельца, только если строка `STOP-AND-RETURN-TO-OWNER` не `no`, строка `OWNER-QUESTION` не `no`, в тексте задач 2–5 для полученного исхода прямо сказано «вопрос владельцу продукта» или «возвращается владельцу продукта», а записи об этом нет (например, в шаге 8 задачи 3 по дорожке VPN не ответил ни один размер пакета UDP, хотя копии прямой дорожки тот же сервер ответил), или решение `OPEN` нельзя закрыть повтором пропущенного шага. Исходы, о которых владельцу только сообщается (`A3-unknown-mode` — `FAIL`; `I-<метка>-direct` — `FAIL` при `PASS I-<метка>`), план 1 не задерживают.
