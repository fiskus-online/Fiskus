---
title: Dane
sidebar_position: 7
description: Priznanie k DPH, kontrolný výkaz a súhrnný výkaz ako XML pre eDane; kalendár termínov.
tags: [dph, dane]
---

# Dane

Priznanie aj výkazy sa **počítajú z dokladov** za zvolené obdobie (mesiac
alebo štvrťrok). Každý riadok sa dá rozkliknúť až na doklady, z ktorých
vznikol — na hodine to je najlepší spôsob, ako ukázať, odkiaľ sa čísla
v tlačive berú.

![Priznanie k DPH po riadkoch — každý riadok sa dá rozkliknúť na doklady](../obrazky/dph.webp)
*Priznanie k DPH po riadkoch — každý riadok sa dá rozkliknúť na doklady*


| Tlačivo | Verzia | Výstup |
|---|---|---|
| Priznanie k DPH | DPHv25 | XML pre eDane, náhľad po riadkoch |
| Kontrolný výkaz | KVDPHv25 | XML, časti A.1, A.2, B.1–B.3, C.1, C.2, D.1, D.2; riadny, opravný aj dodatočný |
| Súhrnný výkaz | SVDPHv20 | XML, kódy 0, 1, 2 |

Opravné a dodatočné priznanie: dátum zistenia, rozdiel oproti poslednej
známej povinnosti (r. 36), záznam o podaní.

### Dodatočný kontrolný výkaz

Po podaní v eDane kliknite na **Zaznamenať podanie**. Fiškus si zapamätá
výsledok priznania aj celý kontrolný výkaz, ako bol podaný.

Keď neskôr opravíte doklady za podané obdobie (chýbajúca faktúra, zlá
suma, faktúra, ktorá tam nepatrí), zvoľte druh **Dodatočné**. Kontrolný
výkaz potom ukáže iba opravované riadky:

- **storno** (kód opravy 1) — pôvodný riadok, ktorý sa ruší,
- **nový** (kód opravy 2) — správny alebo chýbajúci riadok.

Zmenená faktúra je teda dvakrát: pôvodná ako storno a správna ako nová.
Súhrnné riadky B.3.1, D.1 a D.2 sa opravujú rovnako. XML pre eDane má druh
„dodatočný“. Po podaní dodatočného výkazu znova kliknite na **Zaznamenať
podanie** — ďalší dodatočný výkaz sa porovná už s ním.

Ak ste podanie vo Fiškuse nezaznamenali (alebo ste ho zaznamenali vo
verzii pred 0.10.10), Fiškus nevie, čo bolo podané, a dodatočný kontrolný
výkaz nevytvorí — opravte ho v eDane.

![Kontrolný výkaz — časti A.1, A.2, B.1 až B.3](../obrazky/kontrolny-vykaz.webp)
*Kontrolný výkaz — časti A.1, A.2, B.1 až B.3*

![Súhrnný výkaz — dodania do iných štátov EÚ](../obrazky/suhrnny-vykaz.webp)
*Súhrnný výkaz — dodania do iných štátov EÚ*


:::warning[Podanie je na vás]
Fiškus pripraví XML, ktoré nahráte na portál finančnej správy. Priamo
nepodáva — podanie vyžaduje vaše prihlásenie na portáli.
:::

## Kalendár termínov

Lehoty pre DPH, kontrolný a súhrnný výkaz, daň z príjmov a daň z motorových
vozidiel — posunuté na pracovný deň. K tomu splatnosti faktúr, pravidelné
faktúry, vlastné opakované termíny (odvody, STK, poistky) a úlohy zo
[zákaziek, ponúk a adresára](../predaj/zakazky.md#zaznamy-a-ulohy), pri autoservise aj STK/EK
a servis [vozidiel zákazníkov](../predaj/zakazky.md#vozidla). Kalendár sa dá
odoberať do mobilu (iCalendar) alebo prepojiť s Google Kalendárom a
Outlookom.

![Kalendár: daňové lehoty, splatnosti a vlastné termíny](../obrazky/kalendar.webp)
*Kalendár: daňové lehoty, splatnosti a vlastné termíny*


V **Nastaveniach → E-mail** zadáte adresu pre **upozornenia na lehoty** a
koľko dní vopred (1 až 14). Fiškus potom pošle e-mail o každom blížiacom sa
daňovom termíne, vlastnom termíne a splatnej prijatej faktúre — raz za termín,
nie skôr ako o 7:00.
