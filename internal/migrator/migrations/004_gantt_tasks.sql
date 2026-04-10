-- Gantt chart tasks: parent tasks (epics) and child tasks (roles).

CREATE TABLE IF NOT EXISTS gantt_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epic_id UUID NOT NULL REFERENCES epics(id) ON DELETE CASCADE,
    role_id UUID REFERENCES roles(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    progress NUMERIC NOT NULL DEFAULT 0
        CHECK (progress >= 0 AND progress <= 100),
    sort_order INT NOT NULL DEFAULT 0,
    is_parent BOOLEAN NOT NULL DEFAULT FALSE,
    parent_task_id UUID REFERENCES gantt_tasks(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_gantt_tasks_epic
    ON gantt_tasks(epic_id);
CREATE INDEX IF NOT EXISTS idx_gantt_tasks_parent
    ON gantt_tasks(parent_task_id);
