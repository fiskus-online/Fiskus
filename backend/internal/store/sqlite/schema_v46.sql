-- zákazky (jednoduché CRM): číslo, zákazník, stav, termíny; doklady cez doc_links
CREATE TABLE jobs (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    number     TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE UNIQUE INDEX jobs_number ON jobs (company_id, number) WHERE number <> '';
