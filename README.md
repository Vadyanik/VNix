# VNix

CLI manager for managing packages in NixOS via marker blocks with `nixos-rebuild` history tracking.

## Commands

| Command | Description |
|---------|----------|
| `vnix init` | Creates project structure: `.vnix/` (config + SQLite), `modules/vnix_packages.nix` with marker blocks |
| `vnix search [--branch <branch>] <pkg>` | Searches for a package via `nix search`, ranks results and pipes through `fzf` |
| `vnix install <pkg...>` | Validates packages and adds them to the `# vnix:start` / `# vnix:end` marker block |
| `vnix rebuild` | Runs `nixos-rebuild`, captures `git diff` before/after, saves result to SQLite |
| `vnix stats` | Shows rebuild analytics: success rate, duration, file changes |

## Project Structure

```
VNix/
├── cmd/vnix/
│   ├── main.go         # CLI dispatcher
│   ├── init.go         # Initialization
│   ├── install.go      # Package installation
│   ├── search.go       # Package search + ranking (nix search + fzf)
│   ├── rebuild.go      # Rebuild + SQLite
│   ├── stats.go        # Statistics
├── .github/workflows/  # CI (build + test)
├── go.mod / go.sum
└── LICENSE
```

## Usage

```bash
vnix init
vnix search firefox
vnix install htop ripgrep
vnix rebuild
vnix stats
```

## Technologies

- **Go 1.25** — pure Go, no CGO, no external CLI frameworks
- **SQLite** (`modernc.org/sqlite`) — rebuild history storage
- **Marker block** — inserting packages into Nix files via marker comments
- **Git diff** — tracking file changes during rebuild

## CI

GitHub Actions: build and test on every push/PR to `main`.

## License

MIT

---

## Functionality / Функционал / Функціонал

Detailed description of every command and its behavior, in English, Russian, and Ukrainian.

---

### English

VNix is a command-line tool for NixOS that keeps user-installed packages in a single managed Nix file delimited by `# vnix:start` / `# vnix:end` marker comments, and records every `nixos-rebuild` run (duration, success/failure, git diff of tracked files) into a local SQLite database for later analytics.

#### `vnix init`

Initializes a VNix project in the current directory. The operation is **idempotent** — existing files are left untouched and only missing pieces are created.

- Creates the `.vnix/` directory (fails if `.vnix` exists as a regular file, not a directory).
- Creates `.vnix/config.json` with three fields:
  - `managed_packages_file` — path to the managed Nix file (default `modules/vnix_packages.nix`);
  - `rebuild_command` — the shell command executed by `vnix rebuild` (default `sudo nixos-rebuild switch --flake .`);
  - `nixpkgs_branch` — the nixpkgs ref used by `vnix search`.
- **Auto-detects the nixpkgs branch** by scanning, in order: `flake.nix`, `flake.lock`, `configuration.nix`, `/etc/nixos/flake.nix`, `/etc/nixos/flake.lock`, `/etc/nixos/configuration.nix`. Two patterns are matched: `github:NixOS/nixpkgs/<ref>` and the flake-lock JSON form (`"owner": "NixOS"` + `"repo": "nixpkgs"` + `"ref": "..."`).
- **Branch normalization**: `unstable` → `nixos-unstable`; a bare `YY.MM` (e.g. `26.05`) → `nixos-26.05`; anything else is used verbatim.
- If detection fails, the branch is **prompted interactively** on stdin.
- Creates the SQLite database `.vnix/stats.db` with the `rebuilds` table and two indexes (`idx_rebuilds_started_at`, `idx_rebuilds_success`).
- Creates `modules/vnix_packages.nix` containing an `environment.systemPackages` list bounded by the `# vnix:start` / `# vnix:end` markers. If the file already exists, its presence is verified for the markers; if the markers are missing the user is told which lines to add.
- Prints instructions reminding the user to wire the module into their NixOS config via `imports = [ ./modules/vnix_packages.nix ];`.

#### `vnix search [--branch <branch>] <query>`

Searches nixpkgs interactively and installs the chosen package(s) in one go.

- **Branch resolution order**: the `--branch` / `-b` flag → `config.json`'s `nixpkgs_branch` → auto-detection (same routine as `init`). The first non-empty value wins, then it is normalized.
- Runs `nix search github:NixOS/nixpkgs/<branch> <query> --json` and parses the JSON output.
- **Attribute path trimming**: leading `legacyPackages.<system>.` and `packages.<system>.` prefixes are stripped so the displayed name is the package attribute the user actually installs.
- **Ranking algorithm** (`scoreSearchResult`, higher = better):
  - exact match on attribute name: **+1000**;
  - exact match on `pname`: **+900**;
  - attribute name starts with query: **+700**;
  - `pname` starts with query: **+600**;
  - attribute name contains query: **+300**;
  - `pname` contains query: **+200**;
  - description contains query: **+50**;
  - penalties for noise attributes: `unwrapped` **−300**, `tests.` **−200**, `python` **−150**, `gnomeExtensions.` **−150**, `vscode-extensions.` **−100**.
  - Ties are broken alphabetically by attribute name (ascending).
- Results are piped into `fzf --multi` as tab-separated columns `attr\tpname\tversion\tdescription`. The user selects one or more packages.
- `fzf` exit code `130` (Ctrl-C / Esc) is treated as a clean cancel, not an error.
- Selected package names are passed straight to `vnix install`, so search + install is a single interaction.

#### `vnix install <pkg...>`

Adds one or more packages to the marker block in `modules/vnix_packages.nix`.

- **Validation** (per package): non-empty after trim; matches `^[A-Za-z0-9._+-]+$`; no duplicates within the same invocation. Any violation aborts the whole command with a precise error.
- Requires the managed file to contain both `# vnix:start` and `# vnix:end` markers, with `start` appearing before `end`; otherwise the command errors out.
- Each new package is inserted on its own line immediately before `# vnix:end`, indented to match the block.
- Packages already present in the block (compared by trimmed line equality) are **skipped** with a notice, not duplicated.
- The file is written back only if at least one new package was added.
- On success the user is prompted to run `vnix rebuild` to apply the changes.

#### `vnix rebuild`

Runs the configured rebuild command and records a full audit record to SQLite.

- Reads `.vnix/config.json` and executes `rebuild_command` through `sh -c`, streaming stdout/stderr live to the terminal.
- Captures `git diff --numstat --no-ext-diff --ignore-submodules=dirty HEAD --` **before** and **after** the rebuild, then computes the delta between the two snapshots:
  - `diff_files_changed` — number of distinct files touched across the two snapshots;
  - `diff_added_lines` / `diff_deleted_lines` — added/deleted lines from the post-rebuild snapshot;
  - `diff_total_lines` — added + deleted.
  Binary files (numstat `-`) are counted as 0 lines.
- Measures `started_at`, `finished_at` (RFC 3339) and `duration_ms`.
- On failure the error message and process exit code are captured; on success the exit code is `NULL`.
- Inserts one row into the `rebuilds` table with all of the above.

#### `vnix stats`

Reads the SQLite history and prints rebuild analytics.

- Aggregate metrics: total rebuilds, successful count, failed count, **success rate %**, **average duration**, timestamp of the last rebuild, total changed files, total added lines, total deleted lines.
- A "Recent rebuilds" table lists the **5 most recent** records (newest first), each showing a `✓` / `✗` marker, start timestamp, duration, and either the `+added -deleted | files: N` diff (for successes) or the exit code (for failures).
- With no records, aggregates print zeros and the recent list is omitted.

#### Configuration file (`.vnix/config.json`)

```json
{
  "managed_packages_file": "modules/vnix_packages.nix",
  "rebuild_command": "sudo nixos-rebuild switch --flake .",
  "nixpkgs_branch": "nixos-unstable"
}
```

#### SQLite schema (`.vnix/stats.db`)

Table `rebuilds`:

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK | autoincrement |
| `started_at` | TEXT | RFC 3339 |
| `finished_at` | TEXT | RFC 3339 |
| `duration_ms` | INTEGER | rebuild duration |
| `success` | INTEGER | 0 / 1 |
| `exit_code` | INTEGER | NULL on success |
| `command` | TEXT | the command that ran |
| `error_message` | TEXT | NULL on success |
| `diff_files_changed` | INTEGER | |
| `diff_added_lines` | INTEGER | |
| `diff_deleted_lines` | INTEGER | |
| `diff_total_lines` | INTEGER | added + deleted |

Indexes: `idx_rebuilds_started_at` on `started_at`, `idx_rebuilds_success` on `success`.

#### Marker blocks

All package edits happen strictly between `# vnix:start` and `# vnix:end` in `modules/vnix_packages.nix`. The marker text is exact and must not be altered; `vnix install` inserts new packages before `# vnix:end` and leaves everything outside the block untouched.

---

### Русский

VNix — консольная утилита для NixOS, которая хранит устанавливаемые пользователем пакеты в одном управляемом Nix-файле, ограниченном маркерными комментариями `# vnix:start` / `# vnix:end`, и записывает каждый запуск `nixos-rebuild` (длительность, успех/провал, git diff по отслеживаемым файлам) в локальную базу SQLite для последующей аналитики.

#### `vnix init`

Инициализирует проект VNix в текущем каталоге. Операция **идемпотентна** — существующие файлы не затрагиваются, создаются только недостающие элементы.

- Создаёт каталог `.vnix/` (завершается ошибкой, если `.vnix` существует как обычный файл, а не каталог).
- Создаёт `.vnix/config.json` с тремя полями:
  - `managed_packages_file` — путь к управляемому Nix-файлу (по умолчанию `modules/vnix_packages.nix`);
  - `rebuild_command` — команда оболочки, выполняемая `vnix rebuild` (по умолчанию `sudo nixos-rebuild switch --flake .`);
  - `nixpkgs_branch` — ref nixpkgs, используемый командой `vnix search`.
- **Автоопределение ветки nixpkgs** по порядку сканирует: `flake.nix`, `flake.lock`, `configuration.nix`, `/etc/nixos/flake.nix`, `/etc/nixos/flake.lock`, `/etc/nixos/configuration.nix`. Сопоставляются два шаблона: `github:NixOS/nixpkgs/<ref>` и JSON-форма flake-lock (`"owner": "NixOS"` + `"repo": "nixpkgs"` + `"ref": "..."`).
- **Нормализация ветки**: `unstable` → `nixos-unstable`; `YY.MM` (например `26.05`) → `nixos-26.05`; прочее используется как есть.
- Если определение не удалось, ветка **запрашивается интерактивно** со stdin.
- Создаёт базу SQLite `.vnix/stats.db` с таблицей `rebuilds` и двумя индексами (`idx_rebuilds_started_at`, `idx_rebuilds_success`).
- Создаёт `modules/vnix_packages.nix` со списком `environment.systemPackages`, ограниченным маркерами `# vnix:start` / `# vnix:end`. Если файл уже существует, проверяется наличие маркеров; при их отсутствии пользователю сообщается, какие строки нужно добавить.
- Выводит напоминание подключить модуль в конфиг NixOS через `imports = [ ./modules/vnix_packages.nix ];`.

#### `vnix search [--branch <branch>] <query>`

Интерактивный поиск по nixpkgs с установкой выбранных пакетов за один шаг.

- **Порядок разрешения ветки**: флаг `--branch` / `-b` → `nixpkgs_branch` из `config.json` → автоопределение (та же процедура, что в `init`). Побеждает первое непустое значение, затем оно нормализуется.
- Выполняет `nix search github:NixOS/nixpkgs/<branch> <query> --json` и разбирает JSON.
- **Обрезка пути атрибута**: лидирующие префиксы `legacyPackages.<system>.` и `packages.<system>.` удаляются, чтобы отображаемое имя было тем атрибутом пакета, который реально устанавливается.
- **Алгоритм ранжирования** (`scoreSearchResult`, больше = лучше):
  - точное совпадение по имени атрибута: **+1000**;
  - точное совпадение по `pname`: **+900**;
  - имя атрибута начинается с запроса: **+700**;
  - `pname` начинается с запроса: **+600**;
  - имя атрибута содержит запрос: **+300**;
  - `pname` содержит запрос: **+200**;
  - описание содержит запрос: **+50**;
  - штрафы за шумовые атрибуты: `unwrapped` **−300**, `tests.` **−200**, `python` **−150**, `gnomeExtensions.` **−150**, `vscode-extensions.` **−100**.
  - При равенстве очков сортировка по имени атрибута по алфавиту (возрастание).
- Результаты передаются в `fzf --multi` как колонки, разделённые табуляцией: `attr\tpname\tversion\tdescription`. Пользователь выбирает один или несколько пакетов.
- Код выхода `fzf` `130` (Ctrl-C / Esc) трактуется как чистая отмена, а не ошибка.
- Выбранные имена пакетов передаются напрямую в `vnix install`, поэтому поиск + установка — это одно взаимодействие.

#### `vnix install <pkg...>`

Добавляет один или несколько пакетов в маркерный блок `modules/vnix_packages.nix`.

- **Валидация** (для каждого пакета): непустой после обрезки пробелов; соответствует `^[A-Za-z0-9._+-]+$`; без дубликатов в рамках одного вызова. Любое нарушение прерывает всю команду с точным сообщением об ошибке.
- Требуется, чтобы в управляемом файле присутствовали оба маркера `# vnix:start` и `# vnix:end`, причём `start` должен идти до `end`; иначе команда завершается ошибкой.
- Каждый новый пакет вставляется отдельной строкой непосредственно перед `# vnix:end` с отступом, соответствующим блоку.
- Пакеты, уже присутствующие в блоке (сравнение по равенству обрезанных строк), **пропускаются** с уведомлением, а не дублируются.
- Файл перезаписывается, только если добавлен хотя бы один новый пакет.
- При успехе пользователю предлагается выполнить `vnix rebuild` для применения изменений.

#### `vnix rebuild`

Запускает настроенную команду пересборки и записывает в SQLite полную запись аудита.

- Читает `.vnix/config.json` и выполняет `rebuild_command` через `sh -c`, транслируя stdout/stderr в терминал в реальном времени.
- Снимает `git diff --numstat --no-ext-diff --ignore-submodules=dirty HEAD --` **до** и **после** пересборки, затем вычисляет разницу между двумя снимками:
  - `diff_files_changed` — число уникальных затронутых файлов по обоим снимкам;
  - `diff_added_lines` / `diff_deleted_lines` — добавленные/удалённые строки из снимка после пересборки;
  - `diff_total_lines` — added + deleted.
  Бинарные файлы (numstat `-`) считаются как 0 строк.
- Фиксирует `started_at`, `finished_at` (RFC 3339) и `duration_ms`.
- При провале сохраняются сообщение об ошибке и код выхода процесса; при успехе код выхода — `NULL`.
- Вставляет одну строку в таблицу `rebuilds` со всеми перечисленными данными.

#### `vnix stats`

Читает историю из SQLite и выводит аналитику по пересборкам.

- Сводные метрики: всего пересборок, успешных, провальных, **процент успеха**, **средняя длительность**, время последней пересборки, всего изменено файлов, всего добавлено строк, всего удалено строк.
- Таблица «Recent rebuilds» выводит **5 последних** записей (от свежих к старым), каждая со маркером `✓` / `✗`, временем старта, длительностью и либо diff-статистикой `+added -deleted | files: N` (для успехов), либо кодом выхода (для провалов).
- При отсутствии записей сводные метки выводятся нулями, а список последних опускается.

#### Файл конфигурации (`.vnix/config.json`)

```json
{
  "managed_packages_file": "modules/vnix_packages.nix",
  "rebuild_command": "sudo nixos-rebuild switch --flake .",
  "nixpkgs_branch": "nixos-unstable"
}
```

#### Схема SQLite (`.vnix/stats.db`)

Таблица `rebuilds`:

| Колонка | Тип | Примечание |
|--------|------|-------|
| `id` | INTEGER PK | autoincrement |
| `started_at` | TEXT | RFC 3339 |
| `finished_at` | TEXT | RFC 3339 |
| `duration_ms` | INTEGER | длительность пересборки |
| `success` | INTEGER | 0 / 1 |
| `exit_code` | INTEGER | NULL при успехе |
| `command` | TEXT | выполненная команда |
| `error_message` | TEXT | NULL при успехе |
| `diff_files_changed` | INTEGER | |
| `diff_added_lines` | INTEGER | |
| `diff_deleted_lines` | INTEGER | |
| `diff_total_lines` | INTEGER | added + deleted |

Индексы: `idx_rebuilds_started_at` по `started_at`, `idx_rebuilds_success` по `success`.

#### Маркерные блоки

Все правки пакетов происходят строго между `# vnix:start` и `# vnix:end` в `modules/vnix_packages.nix`. Текст маркеров неизменен и не должен редактироваться; `vnix install` вставляет новые пакеты перед `# vnix:end` и не трогает всё, что вне блока.

---

### Українська

VNix — консольна утиліта для NixOS, яка зберігає пакунки, що встановлює користувач, у єдиному керованому Nix-файлі, обмеженому маркерними коментарями `# vnix:start` / `# vnix:end`, і записує кожен запуск `nixos-rebuild` (тривалість, успіх/невдачу, git diff за відстежуваними файлами) до локальної бази SQLite для подальшої аналітики.

#### `vnix init`

Ініціалізує проект VNix у поточному каталозі. Операція **ідемпотентна** — наявні файли не зачіпаються, створюються лише відсутні елементи.

- Створює каталог `.vnix/` (завершується помилкою, якщо `.vnix` існує як звичайний файл, а не каталог).
- Створює `.vnix/config.json` з трьома полями:
  - `managed_packages_file` — шлях до керованого Nix-файлу (за замовчуванням `modules/vnix_packages.nix`);
  - `rebuild_command` — команда оболонки, яку виконує `vnix rebuild` (за замовчуванням `sudo nixos-rebuild switch --flake .`);
  - `nixpkgs_branch` — ref nixpkgs, що його використовує `vnix search`.
- **Автовизначення гілки nixpkgs** по порядку сканує: `flake.nix`, `flake.lock`, `configuration.nix`, `/etc/nixos/flake.nix`, `/etc/nixos/flake.lock`, `/etc/nixos/configuration.nix`. Зіставляються два шаблони: `github:NixOS/nixpkgs/<ref>` і JSON-форма flake-lock (`"owner": "NixOS"` + `"repo": "nixpkgs"` + `"ref": "..."`).
- **Нормалізація гілки**: `unstable` → `nixos-unstable`; `YY.MM` (наприклад `26.05`) → `nixos-26.05`; інше використовується як є.
- Якщо визначити не вдалося, гілка **запитується інтерактивно** зі stdin.
- Створює базу SQLite `.vnix/stats.db` з таблицею `rebuilds` та двома індексами (`idx_rebuilds_started_at`, `idx_rebuilds_success`).
- Створює `modules/vnix_packages.nix` зі списком `environment.systemPackages`, обмеженим маркерами `# vnix:start` / `# vnix:end`. Якщо файл уже існує, перевіряється наявність маркерів; за їх відсутності користувачу повідомляється, які рядки треба додати.
- Виводить нагадування підключити модуль до конфігу NixOS через `imports = [ ./modules/vnix_packages.nix ];`.

#### `vnix search [--branch <branch>] <query>`

Інтерактивний пошук nixpkgs із встановленням обраних пакунків за один крок.

- **Порядок визначення гілки**: прапорець `--branch` / `-b` → `nixpkgs_branch` із `config.json` → автовизначення (та сама процедура, що й в `init`). Перемагає перше непорожнє значення, далі воно нормалізується.
- Виконує `nix search github:NixOS/nixpkgs/<branch> <query> --json` і розбирає JSON.
- **Обрізка шляху атрибута**: провідні префікси `legacyPackages.<system>.` і `packages.<system>.` видаляються, щоб показане ім'я було тим атрибутом пакунка, який реально встановлюється.
- **Алгоритм ранжування** (`scoreSearchResult`, більше = краще):
  - точний збіг за ім'ям атрибута: **+1000**;
  - точний збіг за `pname`: **+900**;
  - ім'я атрибута починається з запиту: **+700**;
  - `pname` починається з запиту: **+600**;
  - ім'я атрибута містить запит: **+300**;
  - `pname` містить запит: **+200**;
  - опис містить запит: **+50**;
  - штрафи за шумові атрибути: `unwrapped` **−300**, `tests.` **−200**, `python` **−150**, `gnomeExtensions.` **−150**, `vscode-extensions.` **−100**.
  - За рівності очок сортування за ім'ям атрибута за абеткою (зростання).
- Результати передаються в `fzf --multi` як колонки, розділені табуляцією: `attr\tpname\tversion\tdescription`. Користувач обирає один або кілька пакунків.
- Код виходу `fzf` `130` (Ctrl-C / Esc) трактується як чиста скасування, а не помилка.
- Обрані імена пакунків передаються напряму до `vnix install`, тож пошук + встановлення — це одна взаємодія.

#### `vnix install <pkg...>`

Додає один або кілька пакунків до маркерного блоку `modules/vnix_packages.nix`.

- **Валідація** (для кожного пакунка): непорожній після обрізки пробілів; відповідає `^[A-Za-z0-9._+-]+$`; без дублікатів в одному виклику. Будь-яке порушення перериває всю команду з точною помилкою.
- Потрібно, щоб у керованому файлі були обидва маркери `# vnix:start` і `# vnix:end`, причому `start` має бути до `end`; інакше команда завершується помилкою.
- Кожен новий пакунок вставляється окремим рядком безпосередньо перед `# vnix:end` з відступом, що відповідає блоку.
- Пакунки, вже наявні в блоці (порівняння за рівністю обрізаних рядків), **пропускаються** з повідомленням, а не дублюються.
- Файл перезаписується, лише якщо додано хоч один новий пакунок.
- За успіху користувачу пропонується виконати `vnix rebuild` для застосування змін.

#### `vnix rebuild`

Запускає налаштовану команду перезбірки й записує до SQLite повний запис аудиту.

- Читає `.vnix/config.json` і виконує `rebuild_command` через `sh -c`, транслюючи stdout/stderr у термінал наживо.
- Знімає `git diff --numstat --no-ext-diff --ignore-submodules=dirty HEAD --` **до** і **після** перезбірки, потім обчислює різницю між двома знімками:
  - `diff_files_changed` — кількість унікальних зачеплених файлів за обома знімками;
  - `diff_added_lines` / `diff_deleted_lines` — додані/видалені рядки зі знімка після перезбірки;
  - `diff_total_lines` — added + deleted.
  Бінарні файли (numstat `-`) рахуються як 0 рядків.
- Фіксує `started_at`, `finished_at` (RFC 3339) і `duration_ms`.
- За невдачі зберігаються повідомлення про помилку та код виходу процесу; за успіху код виходу — `NULL`.
- Вставляє один рядок у таблицю `rebuilds` з усіма переліченими даними.

#### `vnix stats`

Читає історію з SQLite і виводить аналітику перезбірок.

- Зведені метрики: усього перезбірок, успішних, невдалих, **відсоток успіху**, **середня тривалість**, час останньої перезбірки, усього змінено файлів, усього додано рядків, усього видалено рядків.
- Таблиця «Recent rebuilds» виводить **5 останніх** записів (від свіжих до старих), кожен із маркером `✓` / `✗`, часом старту, тривалістю та або diff-статистикою `+added -deleted | files: N` (за успіхів), або кодом виходу (за невдач).
- За відсутності записів зведені метки виводяться нулями, а список останніх опускається.

#### Файл конфігурації (`.vnix/config.json`)

```json
{
  "managed_packages_file": "modules/vnix_packages.nix",
  "rebuild_command": "sudo nixos-rebuild switch --flake .",
  "nixpkgs_branch": "nixos-unstable"
}
```

#### Схема SQLite (`.vnix/stats.db`)

Таблиця `rebuilds`:

| Колонка | Тип | Примітка |
|--------|------|-------|
| `id` | INTEGER PK | autoincrement |
| `started_at` | TEXT | RFC 3339 |
| `finished_at` | TEXT | RFC 3339 |
| `duration_ms` | INTEGER | тривалість перезбірки |
| `success` | INTEGER | 0 / 1 |
| `exit_code` | INTEGER | NULL за успіху |
| `command` | TEXT | виконана команда |
| `error_message` | TEXT | NULL за успіху |
| `diff_files_changed` | INTEGER | |
| `diff_added_lines` | INTEGER | |
| `diff_deleted_lines` | INTEGER | |
| `diff_total_lines` | INTEGER | added + deleted |

Індекси: `idx_rebuilds_started_at` за `started_at`, `idx_rebuilds_success` за `success`.

#### Маркерні блоки

Усі правки пакунків відбуваються строго між `# vnix:start` і `# vnix:end` у `modules/vnix_packages.nix`. Текст маркерів сталий і не повинен редагуватися; `vnix install` вставляє нові пакунки перед `# vnix:end` і не зачіпає все поза блоком.
