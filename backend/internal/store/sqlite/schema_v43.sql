-- vyplnené tlačivá a vzory (hodnoty polí a položky ako JSON)
CREATE TABLE form_docs (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    data       TEXT NOT NULL
);
CREATE INDEX form_docs_company ON form_docs (company_id);
