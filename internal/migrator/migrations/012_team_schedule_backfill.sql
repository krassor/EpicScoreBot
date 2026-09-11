-- Настройка команды «не занимать промежутки расписания, оставшиеся в
-- прошлом» — запрещает планировщику (RecalculateTeamSchedule) подбирать под
-- задачу свободный промежуток в календаре исполнителя/роли, если он целиком
-- или частично лежит раньше текущей даты. См.
-- openspec/changes/backfill-idle-gaps-in-schedule/design.md, Решение 3.
--
-- По умолчанию выключена (FALSE): сразу после применения миграции граница
-- поиска промежутка совпадает с прежней (teamFloor), поэтому выкатка сама
-- по себе не меняет существующие расписания — см. Migration Plan.
ALTER TABLE teams ADD COLUMN IF NOT EXISTS backfill_block_past BOOLEAN NOT NULL DEFAULT FALSE;
