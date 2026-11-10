-- dodacie listy: odberateľ, položky s cenami, stav a faktúra (JSON)
CREATE TABLE deliveries (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    number     TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE UNIQUE INDEX deliveries_number ON deliveries (company_id, number) WHERE number <> '';
