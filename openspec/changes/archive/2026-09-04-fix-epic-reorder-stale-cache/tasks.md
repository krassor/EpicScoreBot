## 1. Frontend: свежие данные при открытии модалки эпиков

- [x] 1.1 В `web/gantt/js/gantt-renderer.js` сделать `openEpicReorderModal()` асинхронной (`async function`); внутри — вместо `const epics = (state.get('epics') || []).filter(e => !e.parent_epic_id)` сделать `const teamId = state.get('selectedTeamId')` и свежий запрос `const data = await apiGet(\`/epics?team_id=${teamId}\`)`, затем `const epics = (data.epics || []).filter(e => !e.parent_epic_id)` — по аналогии с уже существующим паттерном в `loadEpics()` (`web/gantt/js/app.js`) и в `openStoryReorderModal()` (тот же файл, свежий `GET /epics/{epicId}/stories` при каждом открытии)
- [x] 1.2 Обернуть запрос в `try/catch` по аналогии с `openStoryReorderModal()` — при ошибке сети показать `showToast('Не удалось загрузить эпиков: ' + err.message, 'error')` и не открывать модалку (не показывать пустую/некорректную модалку при сбое запроса)
- [x] 1.3 Убедиться, что обработчик клика на кнопке `#btn-reorder-epics` (`setupGanttEvents`, `document.getElementById('btn-reorder-epics')?.addEventListener('click', openEpicReorderModal)`) корректно работает с асинхронной версией функции (addEventListener одинаково поддерживает и sync, и async колбэки — специальных правок не требуется, просто проверить, что ничего не ждёт возвращаемого значения синхронно)
- [x] 1.4 Проверить синтаксис (`node --input-type=module --check < web/gantt/js/gantt-renderer.js`)

## 2. QA

- [x] 2.1 Проверено вручную пользователем в браузере на проде — работает
- [x] 2.2 Проверено вручную пользователем в браузере на проде — работает
- [x] 2.3 Проверено вручную пользователем в браузере на проде — регрессии нет
