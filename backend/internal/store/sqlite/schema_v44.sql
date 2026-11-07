-- uzávierky e-kasy (obrat podľa sadzieb, platby kartou, väzba na PPD a VPD)
CREATE TABLE cash_closings (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    date       TEXT NOT NULL,
    data       TEXT NOT NULL
);
CREATE INDEX cash_closings_company_date ON cash_closings (company_id, date);
