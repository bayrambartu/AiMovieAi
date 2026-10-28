BEGIN;
CREATE TABLE IF NOT EXISTS activities (
  session_id VARCHAR(64) NOT NULL,
  movie_id INTEGER NOT NULL CHECK (movie_id > 0),
  watchlist BOOLEAN NOT NULL DEFAULT FALSE,
  watched BOOLEAN NOT NULL DEFAULT FALSE,
  liked BOOLEAN NOT NULL DEFAULT FALSE,
  rating INTEGER NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 10),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(session_id, movie_id)
);
-- Preserve existing activity rows; movie_id is now a remote TMDB identifier.
ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_movie_id_fkey;
-- Intentionally no CASCADE: unexpected dependencies must abort the migration.
DROP TABLE IF EXISTS movies;
CREATE TABLE IF NOT EXISTS users (
 id BIGSERIAL PRIMARY KEY,
 name VARCHAR(100) NOT NULL,
 surname VARCHAR(100) NOT NULL,
 email VARCHAR(254) NOT NULL UNIQUE CHECK (email = lower(email)),
 password_hash TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS auth_sessions (
 token_hash CHAR(64) PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx ON auth_sessions(user_id);
COMMIT;
