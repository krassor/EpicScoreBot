## Why

Вкладка «📊 Отчёты» в веб-интерфейсе уже показывает таблицы вместимости и квот по команде за выбранные год/квартал (`GET /api/gantt/reports/capacity`), но скачать эти данные файлом нельзя. Существующий PDF-генератор отчёта (Gotenberg) сейчас вызывается только из Telegram-бота — HTTP-эндпоинта для веб-интерфейса нет. Руководителям команд и PO нужно выгружать отчёт за квартал файлом для дальнейшей работы (пересылка, архив, презентации) прямо из веба, не переключаясь в Telegram.

## What Changes

- Backend: новый защищённый (TelegramAuth, без ограничения по роли — как у `/reports/capacity`) HTTP-эндпоинт `GET /api/gantt/reports/export`, принимающий `team_id`, `year`, `quarter`, `format` (`pdf` | `xlsx`), отдающий файл через `Content-Disposition: attachment`.
- Backend: для `format=pdf` — переиспользовать существующий генератор (`internal/report/generator.go`, `epicService.GetReportData`), без изменения его логики/шаблона.
- Backend: для `format=xlsx` — новый генератор XLSX-выгрузки на базе данных, эквивалентных `CapacityReportResponse` (вместимость по ролям, план/факт, квоты, список эпиков с оценками), с использованием новой зависимости `github.com/xuri/excelize/v2` (одобрено пользователем).
- Frontend: во вкладке «📊 Отчёты» (`web/gantt/js/reports-panel.js`) — кнопки «Скачать PDF» и «Скачать Excel», активные при выбранной команде, использующие текущие значения фильтров (команда/год/квартал), инициирующие скачивание файла.
- Зависимости: добавить `github.com/xuri/excelize/v2` в `go.mod`.

## Capabilities

### New Capabilities
- `team-report-export`: выгрузка отчёта по команде за выбранный год/квартал в веб-интерфейсе в форматах PDF и XLSX.

### Modified Capabilities
(нет — существующее поведение `/reports/capacity` и JSON-таблиц не меняется)

## Impact

- **Backend (transport/services)**: новый handler и route в `internal/transport/httpServer/handlers/reports.go` и `internal/transport/httpServer/routers/routers.go`; новый пакет/файл для генерации XLSX (services или report-слой); go.mod/go.sum.
- **Frontend**: `web/gantt/js/reports-panel.js`, разметка вкладки «Отчёты» (кнопки скачивания).
- **QA**: покрытие нового эндпоинта (валидация параметров, форматы, права доступа) и генератора XLSX юнит-тестами.
- **DevOps**: не затронуто — существующая зависимость от Gotenberg для PDF не меняется, XLSX генерируется in-process без внешних сервисов.
