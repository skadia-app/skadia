CREATE TABLE IF NOT EXISTS pmc_daily (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    tss        DOUBLE PRECISION DEFAULT 0,
    ctl        DOUBLE PRECISION DEFAULT 0,
    atl        DOUBLE PRECISION DEFAULT 0,
    tsb        DOUBLE PRECISION DEFAULT 0,
    PRIMARY KEY (user_id, date)
);
