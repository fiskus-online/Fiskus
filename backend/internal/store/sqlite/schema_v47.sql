-- objednávky dodávateľom: číslo, dodávateľ, riadky a prijaté množstvá (JSON)
CREATE TABLE purchase_orders (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    number     TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE UNIQUE INDEX purchase_orders_number ON purchase_orders (company_id, number) WHERE number <> '';
