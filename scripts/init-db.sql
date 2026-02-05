-- Nova Database Schema
-- This script is automatically run when PostgreSQL starts for the first time

-- Functions table
CREATE TABLE IF NOT EXISTS functions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Function versions table
CREATE TABLE IF NOT EXISTS function_versions (
    function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (function_id, version)
);

-- Function aliases table
CREATE TABLE IF NOT EXISTS function_aliases (
    function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (function_id, name)
);

-- Invocation logs table
CREATE TABLE IF NOT EXISTS invocation_logs (
    id TEXT PRIMARY KEY,
    function_id TEXT NOT NULL,
    function_name TEXT NOT NULL,
    runtime TEXT NOT NULL,
    duration_ms BIGINT NOT NULL,
    cold_start BOOLEAN NOT NULL DEFAULT FALSE,
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT,
    input_size INTEGER DEFAULT 0,
    output_size INTEGER DEFAULT 0,
    stdout TEXT,
    stderr TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Runtimes table
CREATE TABLE IF NOT EXISTS runtimes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'available',
    image_name TEXT,
    entrypoint TEXT[],
    file_extension TEXT,
    env_vars JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Config table for system settings
CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_functions_name ON functions(name);
CREATE INDEX IF NOT EXISTS idx_functions_updated_at ON functions(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_function_versions_function_id ON function_versions(function_id);
CREATE INDEX IF NOT EXISTS idx_function_aliases_function_id ON function_aliases(function_id);
CREATE INDEX IF NOT EXISTS idx_invocation_logs_function_id ON invocation_logs(function_id);
CREATE INDEX IF NOT EXISTS idx_invocation_logs_created_at ON invocation_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_invocation_logs_func_time ON invocation_logs(function_id, created_at DESC);

-- Note: Sample functions are now created via API after Nova starts
-- This ensures proper code compilation and storage in function_code table

-- Insert runtimes (10 most common languages, one version each)
INSERT INTO runtimes (id, name, version, status) VALUES
    ('python', 'Python', '3.12.12', 'available'),
    ('node', 'Node.js', '24.13.0', 'available'),
    ('go', 'Go', '1.25.6', 'available'),
    ('rust', 'Rust', '1.93.0', 'available'),
    ('java', 'Java', '21.0.10', 'available'),
    ('ruby', 'Ruby', '3.4.8', 'available'),
    ('php', 'PHP', '8.4.17', 'available'),
    ('dotnet', '.NET', '8.0.23', 'available'),
    ('deno', 'Deno', '2.6.7', 'available'),
    ('bun', 'Bun', '1.3.8', 'available')
ON CONFLICT (id) DO NOTHING;

-- Insert default config values
INSERT INTO config (key, value) VALUES
    ('pool_ttl', '60'),
    ('log_level', 'info')
ON CONFLICT (key) DO NOTHING;

-- Function files table (for multi-file functions)
CREATE TABLE IF NOT EXISTS function_files (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    path TEXT NOT NULL,           -- Relative path, e.g., "lib/utils.py"
    content BYTEA NOT NULL,       -- File content
    is_binary BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(function_id, path)
);

CREATE INDEX IF NOT EXISTS idx_function_files_function_id ON function_files(function_id);

-- Add entry_point column to functions table for multi-file support
-- ALTER TABLE functions ADD COLUMN IF NOT EXISTS entry_point TEXT;

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO nova;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO nova;
