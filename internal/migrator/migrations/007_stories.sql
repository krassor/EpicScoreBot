-- 1. Добавить parent_epic_id в таблицу epics
ALTER TABLE epics ADD COLUMN IF NOT EXISTS parent_epic_id UUID REFERENCES epics(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_epics_parent ON epics(parent_epic_id) WHERE parent_epic_id IS NOT NULL;

-- 2. Миграция данных: для каждого существующего эпика создать одну сторю
DO $$
DECLARE
    r RECORD;
    new_story_id UUID;
BEGIN
    FOR r IN 
        SELECT id, number, name, description, team_id, status, final_score, year, quarter, type 
        FROM epics 
        WHERE parent_epic_id IS NULL 
          AND id NOT IN (SELECT DISTINCT parent_epic_id FROM epics WHERE parent_epic_id IS NOT NULL)
    LOOP
        new_story_id := gen_random_uuid();
        
        INSERT INTO epics (id, number, name, description, team_id, status, final_score, year, quarter, type, parent_epic_id, created_at, updated_at)
        VALUES (new_story_id, r.number || '-S1', r.name, r.description, r.team_id, r.status, r.final_score, r.year, r.quarter, r.type, r.id, NOW(), NOW());
        
        UPDATE epic_scores SET epic_id = new_story_id WHERE epic_id = r.id;
        UPDATE epic_role_scores SET epic_id = new_story_id WHERE epic_id = r.id;
        UPDATE risks SET epic_id = new_story_id WHERE epic_id = r.id;
        UPDATE epic_evaluation_roles SET epic_id = new_story_id WHERE epic_id = r.id;
    END LOOP;
END $$;
