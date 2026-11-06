-- fotka bločku k pohybu domácnosti (jedna na pohyb)
CREATE TABLE hh_photos (
    entry_id     INTEGER PRIMARY KEY,
    company_id   INTEGER NOT NULL REFERENCES companies(id),
    content_type TEXT NOT NULL,
    data         BLOB NOT NULL
);
CREATE INDEX hh_photos_company ON hh_photos (company_id);
