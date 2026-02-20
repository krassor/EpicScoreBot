-- Initial migration: create all tables for EpicScoreBot

-- Teams
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Roles
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT ''
);

-- Seed initial roles
INSERT INTO
    roles (name, description)
VALUES (
        'IT-лидер',
        'IT-лидер команды'
    ),
    (
        'Аналитик',
        'Бизнес/системный аналитик'
    ),
    (
        'BE разработчик',
        'Backend разработчик'
    ),
    (
        'FE разработчик',
        'Frontend разработчик'
    ),
    (
        'Mobile разработчик',
        'Мобильный разработчик'
    ),
    ('Тестировщик', 'QA инженер')
ON CONFLICT (name) DO NOTHING;

-- Users
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    / BIGINT NOT NULL UNIQUE,
    weight INT NOT NULL DEFAULT 100 CHECK (
        weight >= 0
        AND weight <= 100
    ),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- User-Team relation (many-to-many)
CREATE TABLE IF NOT EXISTS user_teams (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, team_id)
);

-- User-Role relation (many-to-many)
CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

-- Epics
CREATE TABLE IF NOT EXISTS epics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    number TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'NEW',
    final_score NUMERIC,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_epics_team_status ON epics (team_id, status);

-- Risks
CREATE TABLE IF NOT EXISTS risks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    description TEXT NOT NULL,
    epic_id UUID NOT NULL REFERENCES epics (id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'NEW',
    weighted_score NUMERIC,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_risks_epic ON risks (epic_id);

-- Epic scores (one per user per epic)
CREATE TABLE IF NOT EXISTS epic_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    epic_id UUID NOT NULL REFERENCES epics (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    score INT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (epic_id, user_id)
);

-- Aggregated role scores for epic
CREATE TABLE IF NOT EXISTS epic_role_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    epic_id UUID NOT NULL REFERENCES epics (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    weighted_avg NUMERIC NOT NULL,
    UNIQUE (epic_id, role_id)
);

-- Risk scores (one per user per risk)
CREATE TABLE IF NOT EXISTS risk_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    risk_id UUID NOT NULL REFERENCES risks (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    probability INT NOT NULL CHECK (
        probability >= 1
        AND probability <= 4
    ),
    impact INT NOT NULL CHECK (
        impact >= 1
        AND impact <= 4
    ),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (risk_id, user_id)
);