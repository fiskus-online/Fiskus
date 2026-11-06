---
title: Mzdy
sidebar_position: 8
description: Zamestnanci a dohodári, výpočet mzdy, odvodov, preddavku na daň a daňového bonusu, výplatné pásky.
tags: [mzdy, zamestnanci]
---

# Mzdy

Fiškus vypočíta mzdy zamestnancov v pracovnom pomere aj odmeny z dohôd:
poistné do Sociálnej poisťovne po fondoch, zdravotné poistenie, preddavok
na daň, daňový bonus na deti, čistú mzdu a cenu práce. Počíta roky 2025
a 2026 podľa sadzieb a limitov platných v danom roku.

## Zamestnanci

V časti **Mzdy → Zamestnanci** zadáte pri každom:

| Údaj | Na čo sa použije |
|---|---|
| Druh vzťahu | pracovný pomer, DoPČ, DoVP, DoBPŠ — určuje fondy sociálneho poistenia |
| Pravidelný mesačný príjem (dohody) | dohoda s pravidelnou odmenou má poistenie ako pracovný pomer |
| Mesačná mzda alebo hodinová sadzba | základná mzda; pri hodinovej sa násobí odpracovanými hodinami |
| Zdravotná poisťovňa | súčty zdravotného poistenia podľa poisťovní |
| Dôchodok | starobný: bez invalidného poistenia a poistenia v nezamestnanosti; invalidný nad 70 %: bez poistenia v nezamestnanosti |
| ZŤP | znížená sadzba zdravotného poistenia |
| Odvodová výnimka 200 € | študent na DoBPŠ a dôchodca na dohode — sociálne poistenie až zo sumy nad 200 € (iba na jednej dohode) |
| Vyhlásenie na NČZD | nezdaniteľná časť základu dane sa uplatní mesačne |
| Daňový bonus | deti s rodným číslom; vek a mesiace sa určia ako v daňovom priznaní |

## Výplatná listina

V časti **Mzdy** je výplatná listina mesiaca. Kliknutím na zamestnanca
zadáte mesačné údaje — odpracované hodiny, inú základnú mzdu (časť
mesiaca), odmeny, náhradu mzdy za dovolenku alebo sviatok, náhradu príjmu
pri PN a zrážky — a uvidíte celú výplatnú pásku.

Hore sú súčty na úhradu: **Sociálna poisťovňa** (zamestnanci aj firma),
**zdravotné poisťovne** zvlášť, **daňový úrad** (preddavky po daňovom
bonuse) a **výplaty**. Výplatné pásky vytlačíte alebo uložíte do PDF, dve
na hárok A4.

**Uzavrieť mesiac** uloží výsledky tak, ako sú — po zmene mzdy alebo údajov
zamestnanca sa uzavretý mesiac neprepočíta. Otvoriť ho môžete s uvedením
dôvodu (zapíše sa do auditu). Zamestnanca s uzavretou mzdou nezmažete,
zadáte mu koniec pracovného vzťahu.

## Ako Fiškus počíta

- **Sociálne poistenie** po fondoch z hrubej mzdy, najviac z maximálneho
  vymeriavacieho základu (2026: 16 764 €; úrazové bez maxima); každý fond
  sa zaokrúhľuje na eurocent nadol.
- **Zdravotné poistenie** 2026: zamestnanec 5 %, firma 11 % (ZŤP 2,5 %
  a 5,5 %); 2025: 4 % a 11 %.
- **Preddavok na daň** zo základu (hrubá mzda + náhrada pri PN − poistné
  zamestnanca) po odpočítaní 1/12 nezdaniteľnej časti (2026: 497,23 €, nad
  2 173,59 € sa znižuje); pásma 19, 25, 30 a 35 % sú 1/12 ročných.
- **Daňový bonus** 100 € na dieťa do 15 rokov a 50 € do 18 rokov, nad
  základom 2 286 € (2026) sa znižuje o 1/10 rozdielu, spolu najviac 29 až
  64 % základu podľa počtu detí. Ak je bonus vyšší ako preddavok, rozdiel
  vyplatí zamestnávateľ.

:::info[Čo zatiaľ chýba]
Výkazy pre Sociálnu poisťovňu a zdravotné poisťovne (XML), mesačný prehľad
a ročné hlásenie pre finančnú správu, ročné zúčtovanie, výpočet náhrady
pri PN a dovolenke z priemerného zárobku a zaúčtovanie miezd. Pribudnú
v ďalších verziách — sumy z výplatnej listiny zatiaľ prepíšete do
formulárov poisťovní a finančnej správy.
:::
