# VNix

CLI-менеджер для управления пакетами в NixOS через marker block с отслеживанием истории `nixos-rebuild`.

## Команды

| Команда | Описание |
|---------|----------|
| `vnix init` | Создаёт структуру проекта: `.vnix/` (config + SQLite), `modules/vnix_packages.nix` с marker block |
| `vnix install <pkg...>` | Валидирует пакеты и добавляет их в marker block `# vnix:start` / `# vnix:end` |
| `vnix rebuild` | Запускает `nixos-rebuild`, снимает `git diff` до/после, сохраняет результат в SQLite |
| `vnix stats` | Показывает аналитику пересборок: успешность, длительность, изменения файлов |
| `vnix search` | *(в разработке)* |

## Структура проекта

```
VNix/
├── cmd/vnix/
│   ├── main.go         # CLI диспетчер
│   ├── init.go         # Инициализация
│   ├── install.go      # Установка пакетов
│   ├── rebuild.go      # Пересборка + SQLite
│   ├── stats.go        # Статистика
│   └── search.go       # Заглушка
├── internal/           # Пакеты (в разработке)
├── .github/workflows/  # CI (build + test)
├── go.mod / go.sum
└── LICENSE
```

## Использование

```bash
vnix init
vnix install htop ripgrep
vnix rebuild
vnix stats
```

## Технологии

- **Go 1.25** — чистый Go, без CGO, без внешних CLI-фреймворков
- **SQLite** (`modernc.org/sqlite`) — хранение истории пересборок
- **Marker block** — вставка пакетов в Nix-файл через комментарии-маркеры
- **Git diff** — отслеживание изменений файлов при пересборке

## CI

GitHub Actions: сборка и тестирование на каждый push/PR в `main`.

## Лицензия

MIT
