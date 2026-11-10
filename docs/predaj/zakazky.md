---
title: Zákazky
slug: zakazky
sidebar_position: 4
description: Ponuky, faktúry, dodacie listy a náklady jednej práce pokope so ziskom a maržou.
tags: [predaj, zakazky]
---

# Zákazky

Zákazka spája všetko, čo patrí k jednej práci pre zákazníka: dopyt,
cenové ponuky, faktúry, dodacie listy aj prijaté faktúry za materiál
a subdodávky. Z prepojených dokladov Fiškus sám spočíta, koľko ste
ponúkli, vyfakturovali, koľko vám zákazník zaplatil, aké boli náklady
a koľko na zákazke zostalo.

Zákazky nájdete v menu **Predaj → Zákazky**.

## Nová zákazka

Tlačidlo *Nová zákazka* (alebo kláves <kbd>N</kbd>) otvorí formulár:

- **Názov zákazky** – napríklad „Rekonštrukcia rozvádzača – sušička obilia“.
- **Zákazník** – vyberiete z adresára.
- **Plánovaná cena** – čo za zákazku očakávate (u platiteľa DPH bez DPH).
- **Stav** – *Dopyt*, *Ponuka*, *V realizácii*, *Dokončená* alebo *Stratená*.
- **Začiatok** a **Termín dokončenia**.
- **Poznámka**.

Po uložení dostane zákazka číslo z vlastného radu (Z2026001, Z2026002…).
Zákazku založíte aj priamo z kontaktu v adresári tlačidlom *Nová zákazka*.

## Doklady k zákazke

V detaile zákazky sú tlačidlá **Nový doklad k zákazke: Ponuka / Faktúra**.
Nová ponuka alebo faktúra má predvyplneného zákazníka a po uložení sa
k zákazke prepojí sama.

Už existujúce doklady pridáte v časti **Súvisiace doklady** tlačidlom
*Prepojiť doklad* – rovnako ako pri [faktúrach](faktury.md#suvisiace-doklady).
K zákazke tak patria aj prijaté faktúry (náklady) a tlačivá ako dodací
list či preberací protokol. Prepojenie zrušíte krížikom, doklad tým
nezmažete.

![Detail zákazky s ekonomikou a dokladmi](../obrazky/zakazka.webp)
*Detail zákazky s ekonomikou a dokladmi*

## Ekonomika zákazky

Hore v detaile je prehľad:

| Údaj | Čo obsahuje |
|---|---|
| Ponúknuté | vystavené, prijaté a vyfakturované cenové ponuky |
| Vyfakturované | vystavené faktúry a dobropisy |
| Uhradené | čo zákazník zaplatil, pod tým suma, ktorá ešte ostáva na úhradu |
| Náklady | prijaté faktúry prepojené so zákazkou |
| Zisk | vyfakturované mínus náklady, pod tým marža v percentách |

U platiteľa DPH sú ponuky, fakturácia a náklady **bez DPH**, úhrady s DPH.
Neplatiteľ vidí všetko vrátane DPH. Koncepty sa nerátajú. Zálohové faktúry
sa do fakturácie nerátajú – zaráta sa až vyúčtovacia faktúra, aby suma
nebola dvakrát.

## Zoznam zákaziek

Zoznam má záložky *Bežiace*, *Všetky*, *Po termíne*, *Dokončené* a
*Stratené*. Bežiaca zákazka, ktorej termín dokončenia už prešiel, je
označená červeno. Hore je súčet plánovanej ceny bežiacich zákaziek, suma
na úhradu od zákazníkov a zisk. Ako každý zoznam vo Fiškuse má aj tento
hľadanie a export do Excelu a CSV.

![Zoznam zákaziek](../obrazky/zakazky.webp)
*Zoznam zákaziek*

## Zákazky a doklady pri kontakte

V adresári sa pri kontakte zobrazujú jeho zákazky a posledné faktúry
a ponuky. Kliknutím ich otvoríte.

!!! note "Zákazka v účtovníctve je niečo iné"
    V podvojnom účtovníctve je ešte analytická *zákazka* v účtovom rozvrhu
    (kód, ktorý sa pridáva k zaúčtovaniu). Tá slúži na rozbor nákladov a
    výnosov v účtovníctve. Zákazky v predaji sú jednoduchá evidencia práce
    pre zákazníka a s účtovaním nesúvisia.
