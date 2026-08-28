## 1. Devops: скрипт деплоя по образу

- [x] 1.1 Создать `scripts/deploy/deploy_image.sh <version>`: определить
      текущий тег образа запущенного контейнера
      `app-backend-service-epic-score-bot` (для отката), `docker pull
      <repo>:<version>`, `export VERSION=<version>` + `docker compose up
      -d --no-deps app-backend-service-epic-score-bot`, healthcheck-цикл
      на `http://localhost:8080/ping` (до 30 попыток, по образцу
      `scripts/deploy/deploy.sh`), логирование в
      `/var/log/epicscorebot-deploy.log`. Проверить: `bash -n
      scripts/deploy/deploy_image.sh` (синтаксис) и ручной прогон на
      тестовом/локальном docker-compose с уже собранным локально образом
      под тем же именем.
- [x] 1.2 Добавить в скрипт откат на предыдущий тег при неуспешном
      healthcheck: `docker pull` сохранённого предыдущего тега (на
      случай, если локально уже удалён) → `export VERSION=<предыдущий>`
      → `docker compose up -d --no-deps ...` → повторный healthcheck;
      обработать случай отсутствия предыдущего тега (первый деплой по
      схеме) — выйти с ошибкой, не пытаясь откатиться. Проверить ручным
      прогоном с намеренно нерабочим "новым" образом (например, без
      доступного порта) — что откат срабатывает.
- [x] 1.3 Сделать `scripts/deploy/deploy_image.sh` исполняемым
      (`chmod +x`) и закоммитить.

## 2. Devops: GitHub Actions workflow

- [x] 2.1 Создать `.github/workflows/release.yml` с триггером
      `on: push: tags: ['v*.*.*']` и джобой `build-and-push`: checkout,
      `docker/setup-buildx-action`, `docker/login-action` (секреты
      `DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN`), извлечение версии из
      `github.ref_name` (без префикса `v`), `docker/build-push-action` с
      тегами `<DOCKERHUB_USERNAME>/epicscorebot:<version>` и
      `...:latest`. Проверить синтаксис через `actionlint` (если
      установлен) или ручной review YAML.
- [x] 2.2 Добавить джобу `deploy` с `needs: build-and-push`: SSH на VPS
      (`appleboy/ssh-action`, секреты `VPS_HOST`/`VPS_USER`/`VPS_SSH_KEY`),
      команда на сервере — `cd /home/EpicScoreBot && git pull origin main
      && bash scripts/deploy/deploy_image.sh <version>` (версия — из
      того же `github.ref_name`, без префикса `v`).
- [x] 2.3 Задокументировать в README или `.agents/tasks/task_devops.md`
      требуемые GitHub Secrets (`DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`,
      `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`) и обязательный одноразовый
      ручной шаг: на VPS в `docker-compose.yml` сервис
      `app-backend-service-epic-score-bot` должен ссылаться на
      `image: <DOCKERHUB_USERNAME>/epicscorebot:${VERSION}` вместо
      `build: context: .` — без этого шага джоба `deploy` не обновит
      контейнер на нужный образ.

## 3. QA / проверка

- [ ] 3.1 Прогнать пайплайн на тестовом теге (например `v0.0.1-test` —
      либо временно ослабить паттерн, либо использовать реальный первый
      релизный тег) и убедиться: образ появился в Docker Hub с обоими
      тегами, джоба `deploy` запустилась только после успешного push,
      контейнер на VPS обновился и прошёл healthcheck.
- [ ] 3.2 Проверить сценарий отката: смоделировать неудачный healthcheck
      (например, временно испортить конфиг в новом образе) и убедиться,
      что `deploy_image.sh` вернул контейнер на предыдущую версию и
      завершился с ненулевым кодом (джоба `deploy` помечена как failed).
