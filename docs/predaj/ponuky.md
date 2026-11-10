---
title: Cenové ponuky a pravidelná fakturácia
slug: ponuky
sidebar_position: 3
description: Ponuka s prijatím cez odkaz a faktúry, ktoré sa vystavia samy.
tags: [predaj, ponuky]
---

# Cenové ponuky a pravidelná fakturácia

## Cenová ponuka

Ponuka má vlastný číselný rad (CP), platnosť a stavy *odoslaná*, *prijatá*,
*odmietnutá*, *po platnosti*. Do DPH sa nezapočíta. Na serveri s verejnou
adresou ju zákazník prijme alebo odmietne cez tajný odkaz a môže pripísať
číslo svojej objednávky. Prijatá ponuka sa jedným klikom zmení na koncept
faktúry.

Vystavenú ponuku (aj prijatú či odmietnutú) môžete upravovať, kým nie je
vyfakturovaná: zmeny uložíte tlačidlom *Uložiť zmeny*, číslo a stav ostanú
a úprava sa zapíše do auditu. Ak ste ponuku už poslali, pošlite zákazníkovi
upravenú. Novú verziu s novým číslom vytvorí *Kópia*. Samotné číslo
zmeníte tlačidlom *Zmeniť číslo*.

Ak je ponuka vyfakturovaná faktúrou, ktorá už existuje (vystavili ste ju
mimo ponuky alebo ste ponuky preniesli z iného programu), použite *Už
vyfakturovaná…* a vyberte faktúru. Ponuka dostane stav *vyfakturovaná* a
faktúra sa ukáže v jej súvisiacich dokladoch.

### Čiastková fakturácia

Väčšiu zákazku často fakturujete po častiach – napríklad 50 % po dodaní
rozvádzača a zvyšok po revízii. Na vystavenej alebo prijatej ponuke
kliknite na **Čiastková faktúra…**:

- zadajte **percento** ponuky (Fiškus navrhne množstvá všetkých položiek),
- alebo vyplňte **množstvá po položkách** (napríklad iba materiál).

Vznikne koncept faktúry s týmito množstvami a textom „Čiastková faktúra
č. 1 podľa cenovej ponuky …“. Ponuka si pamätá, čo už je vyfakturované,
a ukazuje stav *čiastočne vyfakturovaná*; viac, ako ostáva, sa
vyfakturovať nedá. Tlačidlo **Faktúra za zvyšok** vystaví záverečnú
faktúru so všetkým, čo ostalo, a ponuka bude *vyfakturovaná*.

Ak koncept čiastkovej faktúry zmažete alebo faktúru stornujete, jej
množstvá sa do ponuky vrátia. Po prvej čiastkovej faktúre sa ponuka už
nedá upraviť ani označiť ako odmietnutá.

![Čiastková faktúra k ponuke](../obrazky/ciastkova-faktura.webp)
*Čiastková faktúra k ponuke*

Pod ponukou sú [záznamy a úlohy](zakazky.md#zaznamy-a-ulohy) – napríklad
úloha *ozvať sa k ponuke* s termínom, ktorá sa ukáže v kalendári.

Menej časté akcie ponuky (kópia, zmena čísla, dodací list, „už
vyfakturovaná“, odmietnutá) sú v ponuke **Viac**.

Keď tovar alebo prácu z ponuky odovzdávate po častiach, tlačidlo
*Dodací list* pripraví koncept [dodacieho listu](dodacie-listy.md)
s odberateľom a položkami ponuky; faktúru potom urobíte z dodacích listov.

![Cenové ponuky so stavmi a úspešnosťou](../obrazky/ponuky.webp)
*Cenové ponuky so stavmi a úspešnosťou*

![Ponuka s odkazom pre zákazníka a vytvorením faktúry](../obrazky/ponuka.webp)
*Ponuka s odkazom pre zákazníka a vytvorením faktúry*


## Pravidelná fakturácia

Šablóna faktúry s intervalom (mesačne, štvrťročne, polročne, ročne) a
koncom. V texte možno použiť `{obdobie}`, `{mesiac}`, `{rok}` a
`{minule_obdobie}`. Fiškus faktúru vytvorí a vystaví sám — pri štarte a
každú hodinu — a ak chcete, aj ju pošle e-mailom.

![Šablóny pravidelných faktúr a koncepty na kontrolu](../obrazky/pravidelna-fakturacia.webp)
*Šablóny pravidelných faktúr a koncepty na kontrolu*


!!! note "Dobehnuté obdobia"
    Ak program dlhšie nebežal, staršie obdobia sa vytvoria ako koncepty na
    kontrolu, nie ako vystavené faktúry. Odznak v menu ukáže, koľko ich čaká.
