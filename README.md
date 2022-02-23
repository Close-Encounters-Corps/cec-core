# cec-core
Core of the Close Encounters Corps platform 

## PostgreSQL setup
```sql
CREATE TABLE principals (
    id BIGSERIAL NOT NULL PRIMARY KEY,
    is_admin BOOLEAN NOT NULL,
    created_on TIMESTAMP WITH TIME ZONE NOT NULL,
    last_login TIMESTAMP WITH TIME ZONE,
    state VARCHAR(16) NOT NULL
);
CREATE TABLE users (
    id BIGSERIAL NOT NULL PRIMARY KEY,
    principal_id BIGINT NOT NULL REFERENCES principals(id)
);

CREATE TABLE discord_accounts (
    id BIGSERIAL NOT NULL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username VARCHAR(64) UNIQUE NOT NULL,
    api_response JSONB NOT NULL,
    created TIMESTAMP WITH TIME ZONE NOT NULL,
    updated TIMESTAMP WITH TIME ZONE NOT NULL,
    access_token VARCHAR NOT NULL,
    token_type VARCHAR(16),
    token_expires_in TIMESTAMP WITH TIME ZONE NOT NULL,
    refresh_token TEXT NOT NULL
)
CREATE TABLE frontier_accounts (
    id BIGSERIAL NOT NULL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cmdr VARCHAR(64) UNIQUE NOT NULL,
    capi_response JSONB NOT NULL,
    created TIMESTAMP WITH TIME ZONE NOT NULL,
    updated TIMESTAMP WITH TIME ZONE NOT NULL,
    access_token VARCHAR NOT NULL,
    token_type VARCHAR(16),
    token_expires_in TIMESTAMP WITH TIME ZONE NOT NULL,
    refresh_token TEXT NOT NULL
);
```