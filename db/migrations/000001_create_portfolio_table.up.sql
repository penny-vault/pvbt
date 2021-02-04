-- Create portfolio tables that stores saved portfolios
BEGIN;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE IF NOT EXISTS portfolio (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    userid VARCHAR(32) NOT NULL,
    name VARCHAR(32) NOT NULL,
    strategy_shortcode VARCHAR(8) NOT NULL,
    arguments JSONB NOT NULL,
    ytd_return FLOAT,
    cagr_since_inception FLOAT,
    credentials JSONB NOT NULL,
    notifications INT NOT NULL DEFAULT 0
);
CREATE INDEX portfolio_userid_idx ON portfolio(userid);
COMMIT;