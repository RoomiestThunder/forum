# Go Forum Project

## Описание

Веб-форум на Go с поддержкой регистрации, аутентификации, публикации постов и комментариев, лайков/дизлайков, фильтрации по категориям и пользователям. Все данные хранятся в SQLite. Проект полностью контейнеризован с помощью Docker.

## Возможности

- Регистрация и вход пользователей (email, username, пароль)
- Хеширование паролей (bcrypt)
- Одна сессия на пользователя (cookie с истечением срока)
- Создание, просмотр, фильтрация и удаление постов
- Комментирование постов
- Лайки и дизлайки для постов и комментариев
- Категории для постов (можно выбрать несколько)
- Фильтрация постов по категориям, своим постам, понравившимся постам
- Просмотр форума и постов без регистрации (только чтение)
- Обработка ошибок и статусов HTTP
- Современный адаптивный интерфейс на Bootstrap 5 (локально или CDN)
- Тесты для основных функций

## Технологии

- Go (net/http, html/template, database/sql)
- SQLite (github.com/mattn/go-sqlite3)
- bcrypt (golang.org/x/crypto/bcrypt)
- UUID (github.com/google/uuid)
- Docker
- Bootstrap 5 (CDN)

## Структура проекта

- `main.go` — основной сервер и роутинг
- `templates/` — HTML-шаблоны страниц
- `static/css/` — стили (forum.css, bootstrap.min.css)
- `forum.db` — база данных SQLite

## Быстрый старт

### С помощью Docker Compose (рекомендуется)

1. Клонируйте репозиторий:
   ```sh
   git clone ...
   cd forum
   ```

2. Запустите с помощью Docker Compose:
   ```sh
   docker compose up
   ```
   
3. Откройте http://localhost:8080 в браузере.

Для остановки используйте `Ctrl+C` или:
```sh
docker compose down
```

### Обычный Docker

1. Соберите и запустите через Docker:
   ```sh
   docker build -f Dockerfile.minimal -t go-forum .
   docker run -p 8080:8080 -v forum_data:/data -e DB_PATH=/data/forum.db go-forum
   ```

2. Откройте http://localhost:8080 в браузере.

### Разработка

Для разработки используйте:
```sh
docker compose -f docker-compose.dev.yml up
```

Это подключит локальные файлы шаблонов и статики для быстрой разработки.

### Makefile команды

Проект включает Makefile для удобства:

```sh
make help         # Показать все доступные команды
make build        # Собрать Go бинарный файл
make run          # Запустить локально  
make docker-build # Собрать Docker образ
make docker-run   # Запустить Docker контейнер
make compose-up   # Запустить с Docker Compose
make compose-down # Остановить Docker Compose
make compose-dev  # Запустить среду разработки
make clean        # Очистить артефакты сборки
```

## Требования

- Docker и Docker Compose
- (Опционально) Go 1.24 для локальной разработки

## Docker файлы

- `Dockerfile` - основной многослойный Dockerfile (может иметь проблемы с сетевой доступностью)
- `Dockerfile.minimal` - минимальный Dockerfile на базе scratch (рекомендуется)  
- `Dockerfile.simple` - простой Dockerfile на базе Alpine
- `docker-compose.yml` - продакшн конфигурация
- `docker-compose.dev.yml` - конфигурация для разработки

## Примеры SQL-запросов

- CREATE: создание таблиц пользователей, постов, комментариев
- INSERT: добавление пользователя, поста, комментария
- SELECT: выборка постов, комментариев, пользователей, фильтрация

## Тестирование

Тесты лежат в *_test.go файлах. Запуск:
```sh
go test ./...
```

## Лицензия

MIT

---

> Проект реализует все требования: регистрация, сессии, категории, лайки/дизлайки, фильтры, SQLite, Docker, обработка ошибок, тесты, современный UI без сторонних JS-фреймворков.
