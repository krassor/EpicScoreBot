## 1. Backend: зависимость и общий агрегатор данных

- [x] 1.1 Добавить `github.com/xuri/excelize/v2` в `go.mod`/`go.sum` (`go get github.com/xuri/excelize/v2`) и проверить, что `go build ./...` проходит
- [x] 1.2 Вынести агрегацию вместимости/квот/эпиков из `GanttHandler.GetCapacityReport` (`internal/transport/httpServer/handlers/reports.go`) в переиспользуемую функцию/метод сервисного слоя, возвращающую `CapacityReportResponse`, и убедиться, что `GET /api/gantt/reports/capacity` продолжает отдавать тот же JSON, что и раньше (регресс существующих тестов, если есть, либо ручная проверка ответа)

## 2. Backend: эндпоинт выгрузки отчёта

- [x] 2.1 Реализовать хендлер `ExportTeamReport` (`GET /api/gantt/reports/export`), принимающий `team_id`, `year`, `quarter`, `format`; валидировать параметры и возвращать ошибки в стандартном формате `writeErrorCode` (400 при отсутствии/некорректности `team_id` или `format`, 404 при несуществующей команде) — см. `specs/team-report-export/spec.md`, требование «Параметры и валидация выгрузки отчёта»
- [x] 2.2 Для `format=pdf` — вызвать существующие `epicService.GetReportData` и `report.Generator.GenerateReport` без изменения их логики, отдать результат с `Content-Type: application/pdf` и `Content-Disposition: attachment; filename=...`
- [x] 2.3 Для `format=xlsx` — реализовать генератор XLSX на базе `excelize`, использующий агрегатор из задачи 1.2 (вместимость по ролям, план/факт, квоты, список эпиков с оценками), отдать с `Content-Type` для XLSX и `Content-Disposition: attachment; filename=...`
- [x] 2.4 Зарегистрировать маршрут `GET /api/gantt/reports/export` в защищённой группе `TelegramAuth` (`internal/transport/httpServer/routers/routers.go`), рядом с `/reports/capacity`, без дополнительного `RoleAuth`
- [x] 2.5 Проверить вручную (или curl с cookie `tg_sys_auth`): PDF и XLSX скачиваются для существующей команды/периода, и для периода без эпиков PDF/XLSX всё равно формируются (см. сценарий «Отчёт за квартал без эпиков»)

## 3. Frontend: кнопки скачивания на вкладке «Отчёты»

- [x] 3.1 Добавить в разметку вкладки «Отчёты» кнопки «Скачать PDF» и «Скачать Excel», disabled при отсутствии выбранной команды
- [x] 3.2 В `web/gantt/js/reports-panel.js` реализовать обработчики кнопок: переход по `GET /api/gantt/reports/export?team_id=&year=&quarter=&format=pdf|xlsx` (ссылка/`window.location`, без fetch+blob — см. `design.md`, решение 5) с текущими значениями фильтров команды/года/квартала
- [x] 3.3 Обновить состояние disabled/enabled кнопок при смене команды/года/квартала синхронно с существующей логикой `loadCapacityReport()`
- [x] 3.4 Проверить в браузере: при выбранной команде кнопки активны и инициируют скачивание файла нужного формата с ожидаемым именем; без выбранной команды — недоступны

## 4. QA

- [x] 4.1 Юнит-тесты хендлера `ExportTeamReport`: обязательные параметры, невалидный/отсутствующий `format`, несуществующий `team_id`, дефолтные год/квартал — все сценарии из `specs/team-report-export/spec.md` покрыты и `go test ./...` проходит
- [x] 4.2 Тест/сверка данных: для одного и того же team_id/year/quarter значения вместимости/квот/эпиков в XLSX совпадают с ответом `GET /api/gantt/reports/capacity` (см. риск «Расхождение данных PDF и XLSX/JSON» в `design.md`)
- [x] 4.3 Прогнать `go tool cover` для нового/изменённого кода и убедиться, что покрытие не хуже принятого в проекте порога (см. `.agents/tasks/task_qa.md`)
