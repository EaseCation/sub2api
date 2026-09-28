-- OpenAI priority is administrator-managed and opt-in for every user.
ALTER TABLE users ADD COLUMN IF NOT EXISTS openai_priority BOOLEAN NOT NULL DEFAULT FALSE;
