-- MorningStar Brain v2.0 - Database initialization
-- All tables with proper indexes for a 4vCPU/8GB VPS

-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- ============================================================================
-- CONVERSATION HISTORY (Level 2 memory)
-- ============================================================================

CREATE TABLE IF NOT EXISTS conversation_history (
    id SERIAL PRIMARY KEY,
    msg_id TEXT DEFAULT '',
    group_jid TEXT NOT NULL,
    sender_jid TEXT NOT NULL,
    sender_name TEXT DEFAULT '',
    message TEXT NOT NULL,
    is_from_bot BOOLEAN DEFAULT false,
    quoted_msg_id TEXT DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_convhist_msgid ON conversation_history(msg_id);
CREATE INDEX IF NOT EXISTS idx_convhist_group_time ON conversation_history(group_jid, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_convhist_sender ON conversation_history(sender_jid, created_at DESC);

-- ============================================================================
-- USER MEMORY (Level 1 - per user knowledge)
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_memory (
    id SERIAL PRIMARY KEY,
    user_jid TEXT NOT NULL,
    group_jid TEXT NOT NULL DEFAULT '',
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_jid, group_jid, key)
);
CREATE INDEX IF NOT EXISTS idx_user_memory_user ON user_memory(user_jid, group_jid);

-- ============================================================================
-- GROUP MEMORY (Level 3)
-- ============================================================================

CREATE TABLE IF NOT EXISTS group_memory (
    id SERIAL PRIMARY KEY,
    group_jid TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(group_jid, key)
);
CREATE INDEX IF NOT EXISTS idx_group_memory_group ON group_memory(group_jid);

-- ============================================================================
-- CONVERSATION SUMMARIES (Level 4 - compression)
-- ============================================================================

CREATE TABLE IF NOT EXISTS conversation_summaries (
    id SERIAL PRIMARY KEY,
    remote_jid VARCHAR(255) NOT NULL,
    summary TEXT NOT NULL,
    message_count INTEGER DEFAULT 0,
    period_start TIMESTAMP WITH TIME ZONE,
    period_end TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_summaries_remote_jid ON conversation_summaries(remote_jid, created_at DESC);

-- ============================================================================
-- MEMBER DETAILS (Additional member info)
-- ============================================================================

CREATE TABLE IF NOT EXISTS member_details (
    id SERIAL PRIMARY KEY,
    jid VARCHAR(255) NOT NULL,
    group_jid VARCHAR(255) NOT NULL,
    push_name VARCHAR(255),
    description TEXT,
    skills TEXT,
    interests TEXT,
    message_count INTEGER DEFAULT 0,
    interaction_score INTEGER DEFAULT 0,
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(jid, group_jid)
);
CREATE INDEX IF NOT EXISTS idx_details_group ON member_details(group_jid);
CREATE INDEX IF NOT EXISTS idx_details_jid ON member_details(jid);

-- ============================================================================
-- MEMBER PROFILES (Custom Names for display)
-- ============================================================================

CREATE TABLE IF NOT EXISTS member_profiles (
    jid TEXT NOT NULL,
    group_jid TEXT NOT NULL,
    custom_name TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (jid, group_jid)
);

-- ============================================================================
-- USER PROFILES (Detailed per-user information across groups)
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_profiles (
    id SERIAL PRIMARY KEY,
    remote_jid TEXT UNIQUE NOT NULL,
    display_name TEXT,
    profession TEXT,
    role TEXT,
    facts TEXT, -- Can be JSONB or a simple text blob of facts
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ============================================================================
-- GROUP FACTS (Key-value store for group-specific knowledge)
-- ============================================================================

CREATE TABLE IF NOT EXISTS group_facts (
    id SERIAL PRIMARY KEY,
    group_jid TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(group_jid, key)
);

-- ============================================================================
-- ROLES DEFINITION
-- ============================================================================

CREATE TABLE IF NOT EXISTS roles (
    role TEXT PRIMARY KEY,
    description TEXT
);

-- ============================================================================
-- MEMBER ROLES (Badges & Permissions)
-- ============================================================================

CREATE TABLE IF NOT EXISTS member_roles (
    jid TEXT NOT NULL,
    group_jid TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (jid, group_jid, role)
);

-- ============================================================================
-- ROLE PERMISSIONS (What each role can do)
-- ============================================================================

CREATE TABLE IF NOT EXISTS role_permissions (
    role TEXT NOT NULL,
    permission TEXT NOT NULL,
    PRIMARY KEY (role, permission)
);

-- ============================================================================
-- MEMBER POINTS (Reputation/Engagement)
-- ============================================================================

CREATE TABLE IF NOT EXISTS member_points (
    jid TEXT NOT NULL,
    group_jid TEXT NOT NULL,
    points INTEGER DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (jid, group_jid)
);

-- ============================================================================
-- MEMBER PROFILE VERSIONS (History of profile changes)
-- ============================================================================

CREATE TABLE IF NOT EXISTS member_profile_versions (
    id SERIAL PRIMARY KEY,
    jid TEXT NOT NULL,
    group_jid TEXT NOT NULL,
    field_name TEXT NOT NULL,
    old_value TEXT,
    new_value TEXT,
    changed_by TEXT,
    changed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS member_interactions (
    id SERIAL PRIMARY KEY,
    group_jid VARCHAR(255) NOT NULL,
    source_jid VARCHAR(255) NOT NULL,
    target_jid VARCHAR(255) NOT NULL,
    interaction_type VARCHAR(50),
    weight INTEGER DEFAULT 1,
    last_interaction TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(group_jid, source_jid, target_jid, interaction_type)
);

-- ============================================================================
-- STICKER TRACKING
-- ============================================================================

CREATE TABLE IF NOT EXISTS sticker_library (
    id SERIAL PRIMARY KEY,
    file_sha256 VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    tags TEXT,
    image_path TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS member_sticker_usage (
    id SERIAL PRIMARY KEY,
    jid VARCHAR(255) NOT NULL,
    sticker_file_sha256 VARCHAR(255) NOT NULL,
    usage_count INTEGER DEFAULT 1,
    last_used TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(jid, sticker_file_sha256)
);

-- ============================================================================
-- MEDIA METADATA
-- ============================================================================

CREATE TABLE IF NOT EXISTS media_metadata (
    id SERIAL PRIMARY KEY,
    remote_jid VARCHAR(255) NOT NULL,
    message_id VARCHAR(255) NOT NULL,
    media_type VARCHAR(50),
    description TEXT,
    summary TEXT,
    file_path TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(message_id)
);

-- ============================================================================
-- TOPICS
-- ============================================================================

CREATE TABLE IF NOT EXISTS topics (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS message_topics (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(255) NOT NULL,
    topic_id INTEGER REFERENCES topics(id),
    relevance_score FLOAT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================================
-- VECTOR MEMORY (RAG)
-- ============================================================================

CREATE TABLE IF NOT EXISTS message_embeddings (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(255) NOT NULL,
    remote_jid VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    embedding vector(768),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_embeddings_remote_jid ON message_embeddings(remote_jid);

-- ============================================================================
-- ACTIVE SESSIONS
-- ============================================================================

CREATE TABLE IF NOT EXISTS active_sessions (
    session_id SERIAL PRIMARY KEY,
    group_jid VARCHAR(255) NOT NULL,
    user_jid VARCHAR(255) NOT NULL,
    session_type VARCHAR(50) NOT NULL,
    state_json TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_active_sessions_group ON active_sessions(group_jid);

-- ============================================================================
-- GROUP SETTINGS
-- ============================================================================

CREATE TABLE IF NOT EXISTS group_settings (
    group_jid VARCHAR(255) PRIMARY KEY,
    welcome_enabled BOOLEAN DEFAULT TRUE,
    antilink_enabled BOOLEAN DEFAULT FALSE,
    antispam_enabled BOOLEAN DEFAULT FALSE,
    antisuppression_enabled BOOLEAN DEFAULT FALSE,
    is_closed BOOLEAN DEFAULT FALSE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================================
-- USER WARNINGS
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_warnings (
    id SERIAL PRIMARY KEY,
    jid VARCHAR(255) NOT NULL,
    group_jid VARCHAR(255) NOT NULL,
    warning_count INTEGER DEFAULT 0,
    last_warning TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(jid, group_jid)
);

-- ============================================================================
-- NOTES & REMINDERS
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_notes (
    id SERIAL PRIMARY KEY,
    user_jid TEXT NOT NULL,
    group_jid TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    remind_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notes_user ON user_notes(user_jid, group_jid);

-- ============================================================================
-- GROUP RULES
-- ============================================================================

CREATE TABLE IF NOT EXISTS group_rules (
    id SERIAL PRIMARY KEY,
    group_jid TEXT NOT NULL,
    rule_text TEXT NOT NULL,
    added_by TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_rules_group ON group_rules(group_jid);
