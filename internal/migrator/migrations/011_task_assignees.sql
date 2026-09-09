-- Пользовательский ввод для ролевых задач диаграммы Ганта: кто закреплён за
-- задачей и на сколько дней вручную сдвинут её плановый старт. Ключ
-- (epic_id, role_id), где epic_id — идентификатор СТОРИ (или самого эпика
-- для legacy-эпиков без сторей) — см.
-- openspec/changes/add-gantt-task-assignees/design.md, Решение 4.
-- Отдельная таблица (а не колонки в gantt_tasks) нужна, чтобы
-- пользовательский ввод переживал перегенерацию задач, которая удаляет и
-- пересоздаёт все строки gantt_tasks (generateTaskRowsForEpic).
CREATE TABLE IF NOT EXISTS task_assignments (
    epic_id           UUID NOT NULL REFERENCES epics (id) ON DELETE CASCADE,
    role_id           UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    -- ON DELETE SET NULL, а не CASCADE: удаление пользователя не должно
    -- молча стирать закрепление вместе со смещением старта из той же
    -- строки. user_id IS NULL означает «автоматически» — то же самое, что
    -- снятое закрепление.
    user_id           UUID     NULL REFERENCES users (id) ON DELETE SET NULL,
    start_offset_days INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (epic_id, role_id)
);

-- Исполнитель листовой (ролевой) задачи диаграммы Ганта — результат расчёта
-- планировщика (как и даты), а не источник истины; единственное исключение —
-- замороженные задачи (Progress > 0 либо ActualEndDate задан), для которых
-- он читается как факт и не пересчитывается (design.md Решение 6).
-- ON DELETE SET NULL: удаление пользователя не должно ронять задачу — она
-- просто уходит в автоматическое распределение при следующем пересчёте.
ALTER TABLE gantt_tasks ADD COLUMN IF NOT EXISTS assignee_id UUID NULL REFERENCES users (id) ON DELETE SET NULL;

-- Перенос ненулевых пользовательских смещений старта
-- (gantt_tasks.start_offset_days) в task_assignments, восстанавливая связь
-- «строка -> стори» тем же сравнением имён, что использует планировщик
-- (recalculateEpicSchedule): имя родительской (стори) задачи должно
-- совпасть с "<number>: <name>" реальной стори. Колонка
-- gantt_tasks.start_offset_days НЕ удаляется — мигратор проекта forward-only,
-- удаление колонки в этой же миграции сделало бы откат релиза невозможным
-- без восстановления БД из бэкапа (design.md Решение 5). Начиная с этой
-- миграции колонка планировщиком больше не читается и не пишется.
INSERT INTO task_assignments (epic_id, role_id, start_offset_days)
SELECT COALESCE(s.id, gt.epic_id), gt.role_id, gt.start_offset_days
FROM gantt_tasks gt
JOIN gantt_tasks p ON p.id = gt.parent_task_id
LEFT JOIN epics s
       ON s.parent_epic_id = gt.epic_id
      AND s.number || ': ' || s.name = p.name
WHERE gt.role_id IS NOT NULL AND gt.start_offset_days <> 0
ON CONFLICT (epic_id, role_id) DO NOTHING;
