-- CRM aktivity: poznámky, telefonáty a úlohy pri kontakte, zákazke či ponuke (JSON)
CREATE TABLE activities (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    day        TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE INDEX activities_company ON activities (company_id, day);
