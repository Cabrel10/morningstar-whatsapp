-- MorningStar Brain - Database initialization
-- Creates the facts table for conversation memory

CREATE TABLE IF NOT EXISTS facts (
    id SERIAL PRIMARY KEY,
    remote_jid VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_facts_remote_jid ON facts(remote_jid);
CREATE INDEX IF NOT EXISTS idx_facts_created_at ON facts(created_at DESC);

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
