## 1. Frontend: разрешить драг родительских баров, заблокировать для листовых

- [x] 1.1 В `web/gantt/js/gantt-renderer.js` (`renderGantt`) сменить `readonly_dates: true` на `readonly_dates: isReadOnly` (синхронно с существующим `readonly` — оба независимо гейтят драг бара внутри библиотеки, проверено на бандле Frappe Gantt 1.2.2: `mousemove`-обработчик двигает бар только при `!options.readonly && !options.readonly_dates`), для `member`-роли (`isReadOnly === true`) драг по-прежнему не работает
- [x] 1.2 Переписать `on_date_change`: для `task._is_parent === false` (ролевая задача) — немедленно отменять визуальное перемещение (тот же паттерн отката, что применяется сегодня для всех задач); для `task._is_parent === true` — реализовано как no-op (уточнение по факту раздела 2: в Frappe Gantt 1.2.2 нет отдельного колбэка «на отпускание», поэтому `on_date_change` для родительских баров нужно доработать до сохранения последней позиции драга в module-level переменную — см. задачу 2.1, это правка того же колбэка, не новый код с нуля)
- [x] 1.3 Проверено вручную пользователем в браузере на проде — работает

## 2. Frontend: reorder по факту отпускания бара эпика/стори

Уточнено по факту реализации раздела 1: в Frappe Gantt 1.2.2 нет отдельного колбэка «на отпускание» (`on_after_date_change` не существует в этой версии — `date_change` стреляет многократно во время драга и ненадёжно на `mouseup`). Момент отпускания определяем собственным `document`-level `mouseup`-слушателем — см. design.md, Decision 2 (обновлено).

- [x] 2.1 Доработать `on_date_change` для `task._is_parent === true` (сейчас no-op, задача 1.2): сохранять `{taskId, newStart, newEnd}` в module-level переменную (`pendingParentDrag`), без какой-либо другой логики (дёшево, вызывается часто)
- [x] 2.2 В `initGanttRenderer` (один раз при инициализации модуля, через `setupParentDragReorder()`, НЕ внутри `renderGantt`) добавлен `document.addEventListener('mouseup', ...)`: если `pendingParentDrag` пуст — ничего не делает; если не пуст — запускает `handleParentDragRelease(drag)` для сохранённой позиции, `pendingParentDrag` сбрасывается в `null` синхронно перед запуском (не после await), т.е. независимо от результата
- [x] 2.3 `getParentLevelNeighbors` вычисляет набор «соседей» того же уровня (эпики: все top-level эпики уже команда-скоупленного `state.tasks`; стори: все стори того же `_parent_id`), сортировка по `_sort_order`; `handleParentDragRelease` определяет новую позицию перетащенного бара по середине нового интервала дат относительно середин интервалов соседей (X-координатный аналог `nextSibling`-поиска в модалке) — если относительный порядок не изменился (`sameIdOrder`), reorder считается несостоявшимся
- [x] 2.4 Если reorder состоялся — `newOrderIds`/`i+1` renumber (аналог `updateModalInputsOrder`) и `apiPut` на `/epics/{id}/reorder` или `/stories/{id}/reorder` для каждого элемента, чей номер изменился (та же семантика вызова, что в `saveOrder()`, эндпоинты и payload не менялись). Если reorder не состоялся — `renderGantt(state.get('tasks'))` возвращает бар на авторитетную позицию, запрос не шлётся
- [x] 2.5 После успешного reorder — `reloadCurrentTeamTasks()`; сетевая ошибка — toast + `renderGantt(state.get('tasks'))` (откат), по аналогии с `on_progress_change`
- [x] 2.6 Проверено вручную пользователем в браузере на проде — работает
- [x] 2.7 Добавлен in-flight guard от гонки при быстром повторном драге: module-level флаг `isParentReorderInFlight`, выставляется в `true` в начале `handleParentDragRelease` (обёрнуто в `try { ... } finally { isParentReorderInFlight = false; }`, сбрасывается при любом исходе — ранний return/успех/ошибка); mouseup-слушатель (`setupParentDragReorder`) при `isParentReorderInFlight === true` не запускает параллельный `handleParentDragRelease`, а откатывает второй драг визуально через `renderGantt(state.get('tasks'))`, как и предписано design.md — без UI-индикации «занято»

## 3. Frontend: инкрементальное обновление вместо полной пересборки

- [x] 3.1 В `renderGantt` — если `ganttChart` уже существует (не первый рендер) и набор задач структурно не изменился (тот же отсортированный список ID — `lastTaskIds`, только даты/прогресс/sort_order отличаются — типичный случай после reorder), использовать `ganttChart.refresh(ganttTasks)` вместо `container.innerHTML='' + new Gantt(...)`; для первого рендера и структурных изменений (добавление/удаление задач, смена команды) — оставлена полная пересборка. Проверено эмпирически на бандле Frappe Gantt 1.2.2: `refresh(tasks)` вызывает `setup_tasks(tasks)` + `change_view_mode()` (без параметров → текущий `view_mode` сохраняется, дополнительный сброс `view_mode` не требуется); `$svg`/`$container` не пересоздаются, поэтому document-level обработчики (`bind_bar_events`), навешиваемые один раз в конструкторе, не дублируются при каждом обновлении — попутно устраняет утечку слушателей `document.mouseup`, которая раньше накапливалась при каждом `new Gantt(...)`. Обнаруженный побочный эффект: `refresh()`→`change_view_mode()`→`render()`→`set_scroll_position(options.scroll_to)` сбрасывает `scrollLeft` к началу диапазона дат, если `scroll_to` не задан — скомпенсировано вручную (сохранение/восстановление `ganttChart.$container.scrollLeft` вокруг вызова `refresh()`)
- [x] 3.2 Убедиться, что `applyPostRenderEnhancements` (факт-маркеры, галочки завершения) корректно перевызывается и после инкрементального `refresh()`, не только после полной пересборки
- [x] 3.3 Проверено вручную пользователем в браузере на проде — работает

## 4. QA

- [x] 4.1 Проверено вручную пользователем в браузере на проде — edge-кейсы не ломают состояние
- [x] 4.2 Проверено вручную пользователем в браузере на проде — права доступа соблюдаются
- [x] 4.3 Проверено вручную пользователем в браузере на проде — регрессии модалки нет

## 5. Фикс дефекта, найденного на проде (v0.1.3): неверный ID в reorder-запросах

См. design.md, Decision 5 — drag эпика/стори возвращал `500 RESCHEDULE_FAILED`, т.к. в URL `/epics/{id}/reorder`/`/stories/{id}/reorder` подставлялся `gantt_tasks.id` вместо `epics.id`.

- [x] 5.1 Backend (`internal/transport/httpServer/handlers/gantt.go`): добавить поле `EpicID string \`json:"epic_id"\`` в структуру `ganttTaskResp`, заполнить его из `domain.GanttTask.EpicID` в обработчике `GetTasks` (аналогично уже существующему `RoleID`) — для всех строк, не только `is_parent`, без условной логики. Проверить `go build ./...`
- [x] 5.2 Frontend (`web/gantt/js/gantt-renderer.js`, `handleParentDragRelease`): при построении URL reorder-запроса использовать `item.epic_id` вместо `id` (значение из `newOrderIds`, т.е. `gantt_tasks.id`) — `item` уже находится через `neighbors.find(...)` для сравнения `sort_order`, дополнительного поиска не требуется. Внутреннюю бухгалтерию (`getParentLevelNeighbors`, `sameIdOrder`, поиск позиции по датам) не менять — она остаётся на `gantt_tasks.id`
- [x] 5.3 Проверено лично: `go build ./...`, `go vet ./...`, `go test ./...` — все пакеты зелёные; `node --input-type=module --check < web/gantt/js/gantt-renderer.js` — синтаксис ок
- [x] 5.4 Проверено вручную пользователем в браузере на проде — 500 больше не возникает, порядок и даты обновляются корректно
