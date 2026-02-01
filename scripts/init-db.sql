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

-- Insert sample functions for testing
INSERT INTO functions (id, name, data, created_at, updated_at)
VALUES
    (
        'fn-hello-python',
        'hello-python',
        '{
            "id": "fn-hello-python",
            "name": "hello-python",
            "runtime": "python",
            "handler": "main.handler",
            "code_path": "/tmp/nova/functions/hello-python/handler.py",
            "memory_mb": 128,
            "timeout_s": 30,
            "min_replicas": 0,
            "mode": "process",
            "env_vars": {}
        }'::jsonb,
        NOW(),
        NOW()
    ),
    (
        'fn-hello-go',
        'hello-go',
        '{
            "id": "fn-hello-go",
            "name": "hello-go",
            "runtime": "go",
            "handler": "main",
            "code_path": "/tmp/nova/functions/hello-go/handler",
            "memory_mb": 128,
            "timeout_s": 30,
            "min_replicas": 0,
            "mode": "process",
            "env_vars": {}
        }'::jsonb,
        NOW(),
        NOW()
    ),
    (
        'fn-hello-node',
        'hello-node',
        '{
            "id": "fn-hello-node",
            "name": "hello-node",
            "runtime": "node",
            "handler": "index.handler",
            "code_path": "/tmp/nova/functions/hello-node/index.js",
            "memory_mb": 256,
            "timeout_s": 30,
            "min_replicas": 0,
            "mode": "process",
            "env_vars": {}
        }'::jsonb,
        NOW(),
        NOW()
    ),
    (
        'fn-api-gateway',
        'api-gateway',
        '{
            "id": "fn-api-gateway",
            "name": "api-gateway",
            "runtime": "rust",
            "handler": "main",
            "code_path": "/tmp/nova/functions/api-gateway/handler",
            "memory_mb": 512,
            "timeout_s": 60,
            "min_replicas": 1,
            "mode": "persistent",
            "env_vars": {"LOG_LEVEL": "debug"}
        }'::jsonb,
        NOW(),
        NOW()
    ),
    (
        'fn-image-processor',
        'image-processor',
        '{
            "id": "fn-image-processor",
            "name": "image-processor",
            "runtime": "python",
            "handler": "processor.handle",
            "code_path": "/tmp/nova/functions/image-processor/processor.py",
            "memory_mb": 1024,
            "timeout_s": 120,
            "min_replicas": 0,
            "mode": "process",
            "env_vars": {"MAX_SIZE": "10485760"}
        }'::jsonb,
        NOW(),
        NOW()
    )
ON CONFLICT (id) DO NOTHING;

-- Insert runtimes
INSERT INTO runtimes (id, name, version, status) VALUES
    -- Python versions
    ('python', 'Python', '3.12', 'available'),
    ('python3.11', 'Python', '3.11', 'available'),
    ('python3.10', 'Python', '3.10', 'available'),
    ('python3.9', 'Python', '3.9', 'available'),
    -- Go versions
    ('go', 'Go', '1.22', 'available'),
    ('go1.21', 'Go', '1.21', 'available'),
    ('go1.20', 'Go', '1.20', 'available'),
    -- Node.js versions
    ('node', 'Node.js', '22.x', 'available'),
    ('node20', 'Node.js', '20.x', 'available'),
    ('node18', 'Node.js', '18.x', 'available'),
    -- Rust versions
    ('rust', 'Rust', '1.76', 'available'),
    ('rust1.75', 'Rust', '1.75', 'available'),
    -- Deno & Bun
    ('deno', 'Deno', '1.40', 'available'),
    ('bun', 'Bun', '1.0', 'available'),
    -- Ruby versions
    ('ruby', 'Ruby', '3.3', 'available'),
    ('ruby3.2', 'Ruby', '3.2', 'available'),
    -- JVM languages
    ('java', 'Java', '21', 'available'),
    ('java17', 'Java', '17', 'available'),
    ('java11', 'Java', '11', 'available'),
    ('kotlin', 'Kotlin', '1.9', 'available'),
    ('scala', 'Scala', '3.3', 'available'),
    -- Other languages
    ('php', 'PHP', '8.3', 'available'),
    ('php8.2', 'PHP', '8.2', 'available'),
    ('dotnet', '.NET', '8.0', 'available'),
    ('dotnet7', '.NET', '7.0', 'available'),
    ('elixir', 'Elixir', '1.16', 'available'),
    ('swift', 'Swift', '5.9', 'available'),
    ('zig', 'Zig', '0.11', 'available'),
    ('lua', 'Lua', '5.4', 'available'),
    ('perl', 'Perl', '5.38', 'available'),
    ('r', 'R', '4.3', 'available'),
    ('julia', 'Julia', '1.10', 'available'),
    ('wasm', 'WebAssembly', 'wasmtime', 'available')
ON CONFLICT (id) DO NOTHING;

-- Insert sample invocation logs (last 24 hours, every 15 minutes)
-- This creates realistic-looking historical data for charts
DO $$
DECLARE
    func RECORD;
    i INTEGER;
    ts TIMESTAMPTZ;
    req_id TEXT;
    is_cold BOOLEAN;
    is_success BOOLEAN;
    duration INTEGER;
BEGIN
    FOR func IN SELECT id, name, data->>'runtime' as runtime FROM functions LOOP
        FOR i IN 0..95 LOOP
            ts := NOW() - (i * INTERVAL '15 minutes');
            req_id := substr(md5(random()::text), 1, 8);
            is_cold := random() < 0.15;
            is_success := random() < 0.95;
            duration := 20 + floor(random() * 200)::INTEGER;

            -- Add some variance: slower during cold starts
            IF is_cold THEN
                duration := duration + 100 + floor(random() * 150)::INTEGER;
            END IF;

            INSERT INTO invocation_logs (
                id, function_id, function_name, runtime, duration_ms,
                cold_start, success, error_message, input_size, output_size, created_at
            ) VALUES (
                req_id,
                func.id,
                func.name,
                func.runtime,
                duration,
                is_cold,
                is_success,
                CASE WHEN NOT is_success THEN 'Execution error: timeout or runtime exception' ELSE NULL END,
                floor(random() * 1024)::INTEGER,
                floor(random() * 4096)::INTEGER,
                ts
            ) ON CONFLICT (id) DO NOTHING;
        END LOOP;
    END LOOP;
END $$;

-- Insert default config values
INSERT INTO config (key, value) VALUES
    ('pool_ttl', '60'),
    ('log_level', 'info')
ON CONFLICT (key) DO NOTHING;

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO nova;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO nova;
