-- záväzky domácnosti: pôžičky, dlhy, exekúcie, pravidelné platby
CREATE TABLE hh_obligations (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    data       TEXT NOT NULL
);
CREATE INDEX hh_obligations_company ON hh_obligations (company_id);
