-- domácnosť (domáce účtovníctvo): účty a pohyby
CREATE TABLE hh_accounts (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    data       TEXT NOT NULL
);
CREATE INDEX hh_accounts_company ON hh_accounts (company_id);
-- source: pôvod importovaného pohybu (prázdny pri ručnom zápise)
CREATE TABLE hh_entries (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    date       TEXT NOT NULL,
    source     TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE INDEX hh_entries_company ON hh_entries (company_id, date);
CREATE UNIQUE INDEX hh_entries_source ON hh_entries (company_id, source) WHERE source <> '';
