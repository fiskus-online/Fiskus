-- krížové prepojenia dokladov (faktúra, ponuka, prijatá faktúra, tlačivo…)
CREATE TABLE doc_links (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    a_kind     TEXT NOT NULL,
    a_id       INTEGER NOT NULL,
    b_kind     TEXT NOT NULL,
    b_id       INTEGER NOT NULL,
    created    TEXT NOT NULL
);
CREATE UNIQUE INDEX doc_links_pair ON doc_links (company_id, a_kind, a_id, b_kind, b_id);
CREATE INDEX doc_links_b ON doc_links (company_id, b_kind, b_id);
