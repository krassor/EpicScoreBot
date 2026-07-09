-- Quarterly planning: add year, quarter, type to epics, create epic_evaluation_roles table and seed PO/BA roles.

-- 1. Add fields to epics
ALTER TABLE epics ADD COLUMN IF NOT EXISTS year INT NOT NULL DEFAULT 2026;
ALTER TABLE epics ADD COLUMN IF NOT EXISTS quarter INT NOT NULL DEFAULT 3 CHECK (quarter >= 1 AND quarter <= 4);
ALTER TABLE epics ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'feature' CHECK (type IN ('feature', 'architecture', 'techdebt'));

-- 2. Create epic_evaluation_roles table
CREATE TABLE IF NOT EXISTS epic_evaluation_roles (
    epic_id UUID NOT NULL REFERENCES epics (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    PRIMARY KEY (epic_id, role_id)
);

-- 3. Seed PO/BA roles
INSERT INTO roles (name, description) VALUES
('PO', 'Product Owner'),
('BA', 'Business Analyst')
ON CONFLICT (name) DO NOTHING;
