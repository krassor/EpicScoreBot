-- team_admins — таблица many-to-many между users и teams, определяющая
-- team-scoped роль admin: пользователь является администратором только тех
-- команд, для которых есть запись в этой таблице (в отличие от роли
-- superadmin, которая остаётся глобальной и конфигурируется через
-- BotConfig.SuperAdmins). Управляется только superadmin.
CREATE TABLE IF NOT EXISTS team_admins (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    assigned_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, team_id)
);
