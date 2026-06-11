-- MorningStar Brain - Database initialization
-- Creates the facts table for conversation memory

-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS facts (
    id SERIAL PRIMARY KEY,
    remote_jid VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_facts_remote_jid ON facts(remote_jid);
CREATE INDEX IF NOT EXISTS idx_facts_created_at ON facts(created_at DESC);

-- Structured member profiles
CREATE TABLE IF NOT EXISTS group_members (
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

CREATE INDEX IF NOT EXISTS idx_members_group ON group_members(group_jid);
CREATE INDEX IF NOT EXISTS idx_members_jid ON group_members(jid);

-- Social Graph: Who interacts with whom
CREATE TABLE IF NOT EXISTS member_interactions (
    id SERIAL PRIMARY KEY,
    group_jid VARCHAR(255) NOT NULL,
    source_jid VARCHAR(255) NOT NULL,
    target_jid VARCHAR(255) NOT NULL,
    interaction_type VARCHAR(50), -- 'reply', 'mention', 'reaction'
    weight INTEGER DEFAULT 1,
    last_interaction TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(group_jid, source_jid, target_jid, interaction_type)
);

-- Sticker usage and semantic library
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

-- Media Metadata (descriptions of images, PDFs, etc.)
CREATE TABLE IF NOT EXISTS media_metadata (
    id SERIAL PRIMARY KEY,
    remote_jid VARCHAR(255) NOT NULL,
    message_id VARCHAR(255) NOT NULL,
    media_type VARCHAR(50), -- 'image', 'pdf', 'audio'
    description TEXT,
    summary TEXT,
    file_path TEXT, -- Local path on SSD
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(message_id)
);

-- Thematic tracking: Topics
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

-- Vector Memory: Embeddings for RAG
CREATE TABLE IF NOT EXISTS message_embeddings (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(255) NOT NULL,
    remote_jid VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    embedding vector(768), -- Dimensions for nomic-embed-text or similar
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_embeddings_remote_jid ON message_embeddings(remote_jid);

-- Conversation summaries for long-term memory
CREATE TABLE IF NOT EXISTS conversation_summaries (
    id SERIAL PRIMARY KEY,
    remote_jid VARCHAR(255) NOT NULL,
    summary TEXT NOT NULL,
    message_count INTEGER DEFAULT 0,
    period_start TIMESTAMP WITH TIME ZONE,
    period_end TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_summaries_remote_jid ON conversation_summaries(remote_jid);

-- Active Sessions (État conversationnel pour jeux, recherches, etc.)
CREATE TABLE IF NOT EXISTS active_sessions (
    session_id SERIAL PRIMARY KEY,
    group_jid VARCHAR(255) NOT NULL,
    user_jid VARCHAR(255) NOT NULL,
    session_type VARCHAR(50) NOT NULL, -- 'game', 'search', 'chat'
    state_json TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_active_sessions_group ON active_sessions(group_jid);
CREATE INDEX IF NOT EXISTS idx_active_sessions_user ON active_sessions(user_jid);

-- Group Configuration & Settings
CREATE TABLE IF NOT EXISTS group_settings (
    group_jid VARCHAR(255) PRIMARY KEY,
    welcome_enabled BOOLEAN DEFAULT TRUE,
    antilink_enabled BOOLEAN DEFAULT FALSE,
    antispam_enabled BOOLEAN DEFAULT FALSE,
    antisuppression_enabled BOOLEAN DEFAULT FALSE,
    is_closed BOOLEAN DEFAULT FALSE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- User Warnings
CREATE TABLE IF NOT EXISTS user_warnings (
    id SERIAL PRIMARY KEY,
    jid VARCHAR(255) NOT NULL,
    group_jid VARCHAR(255) NOT NULL,
    warning_count INTEGER DEFAULT 0,
    last_warning TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(jid, group_jid)
);
