---
title: Peniaze
sidebar_position: 6
description: Bankové výpisy s automatickým párovaním a pokladňa s DPH.
tags: [banka, pokladňa]
---

# Peniaze

## Banka

![Bankový výpis spárovaný s faktúrami podľa VS a sumy](../obrazky/banka.webp)
*Bankový výpis spárovaný s faktúrami podľa VS a sumy*


| Formát výpisu | Poznámka |
|---|---|
| camt.053 (XML) | štandard slovenských bánk |
| GPC | starší formát (ABO) |
| CSV, Excel | stĺpce podľa hlavičky |

Po načítaní Fiškus **spáruje platby s faktúrami** podľa variabilného symbolu
a sumy. Čo si nie je istý, ponúkne ako návrh; zvyšok spárujete ručne alebo
necháte ako nepárovaný pohyb (poplatky, vklady), ktorý sa v peňažnom
denníku zapíše ručne. Čiastočné úhrady sa sčítajú.

**Faktúra v cudzej mene z eurového účtu.** Pri ručnom párovaní eurovej
platby sa v zozname „Iný doklad“ ponúknu aj faktúry v cudzej mene (hľadať sa
dá aj podľa meny, napr. CZK). Fiškus sa opýta, koľko v mene faktúry platba
uhrádza — predvolený je celý zostatok, pri menšej sume ostane faktúra
čiastočne uhradená a čaká na ďalšiu platbu. V podvojnom účtovníctve sa pri
každej platbe odpíše z účtu 311 pomerná časť sumy faktúry v eurách
(posledná platba zvyšok) a rozdiel je kurzový zisk (663) alebo strata (563).

![Eurová platba k faktúre v CZK: koľko korún uhrádza](../obrazky/banka-cudzia-mena.webp)
*Eurová platba k faktúre v CZK: koľko korún uhrádza*

:::info[Prečo párovanie, a nie odškrtnutie „uhradené“]
Úhrada je udalosť s dátumom a sumou. Z nej sa počíta peňažný denník,
kurzový rozdiel aj stav pohľadávok k ľubovoľnému dňu. Faktúra s odškrtnutým
políčkom by nevedela povedať, kedy a koľko prišlo.
:::

## Pokladňa

Príjmové a výdavkové pokladničné doklady (PPD, VPD) s tlačou a sumou slovom.
Fiškus nedovolí, aby zostatok pokladne klesol pod nulu. Pokladničný doklad
môže uhradiť faktúru v hotovosti.

![Pokladňa — príjmové a výdavkové doklady, zostatok](../obrazky/pokladna.webp)
*Pokladňa — príjmové a výdavkové doklady, zostatok*


- **DPH na pokladničných dokladoch** — bloček ako zjednodušená faktúra
  (odpočet, KV B.3.1/B.3.2 podľa limitu 3 000 €), tržba z e-kasy (KV D.1).
- **Viac pokladní** — každá má vlastný zostatok a rad dokladov.
- **Pokladňa v cudzej mene** — kurz ECB ku dňu dokladu.
