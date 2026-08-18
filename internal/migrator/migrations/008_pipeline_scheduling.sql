-- Конвейерное планирование ролей по сторям + прогресс/факт на диаграмме Ганта.
--
-- 1. epics.sort_order — явный порядок очереди: для топ-эпиков (parent_epic_id IS NULL)
--    очередь скоупится по team_id, для сторей (parent_epic_id IS NOT NULL) — по
--    parent_epic_id. Уникальность не требуется (как и у gantt_tasks.sort_order) —
--    при равенстве порядок доопределяется по number.
ALTER TABLE epics ADD COLUMN IF NOT EXISTS sort_order INT;

-- 2. Факт выполнения ролевой задачи: дата фактического завершения (100%) и
--    фактическая трудоёмкость в рабочих днях, фиксируются автоматически при
--    простановке 100% прогресса.
ALTER TABLE gantt_tasks ADD COLUMN IF NOT EXISTS actual_end_date DATE;
ALTER TABLE gantt_tasks ADD COLUMN IF NOT EXISTS actual_effort_days INT;

-- 3. Идемпотентный backfill sort_order для уже существующих строк, где он ещё
--    не проставлен: топ-эпикам — по (year, quarter, number) в рамках team_id;
--    сторям — по number в рамках parent_epic_id.
DO $$
BEGIN
    UPDATE epics e
    SET sort_order = ranked.rn
    FROM (
        SELECT id, ROW_NUMBER() OVER (
            PARTITION BY team_id ORDER BY year, quarter, number
        ) AS rn
        FROM epics
        WHERE parent_epic_id IS NULL AND sort_order IS NULL
    ) ranked
    WHERE e.id = ranked.id;

    UPDATE epics e
    SET sort_order = ranked.rn
    FROM (
        SELECT id, ROW_NUMBER() OVER (
            PARTITION BY parent_epic_id ORDER BY number
        ) AS rn
        FROM epics
        WHERE parent_epic_id IS NOT NULL AND sort_order IS NULL
    ) ranked
    WHERE e.id = ranked.id;
END $$;
