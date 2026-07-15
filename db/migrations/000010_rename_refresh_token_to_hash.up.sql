-- The column stores a SHA-256 hash of the refresh token, never the token
-- itself: a database leak must not yield working credentials. The old name
-- invited storing the raw JWT.
ALTER TABLE refresh_tokens RENAME COLUMN token TO token_hash;
ALTER INDEX uniq_refresh_tokens_token RENAME TO uniq_refresh_tokens_token_hash;
