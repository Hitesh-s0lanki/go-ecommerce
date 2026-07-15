ALTER INDEX uniq_refresh_tokens_token_hash RENAME TO uniq_refresh_tokens_token;
ALTER TABLE refresh_tokens RENAME COLUMN token_hash TO token;
