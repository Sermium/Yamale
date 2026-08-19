# Currencies Yamale carries

Generated from `scripts/currencies/african-currencies.json` — do not hand-edit;
run `python scripts/currencies/generate.py` after changing the table.

Yamale carries **44 African currencies** as stablecoins, plus the native
token **YML**. Every currency is a `u`-prefixed micro-denomination (exponent 6),
so 1 XOF = 1,000,000 `uxof`.

## The hub model

Every currency has exactly one liquidity pool, paired against **YML**. A swap
between two non-YML currencies routes through YML as the hub — a *double hop*
(sell A for YML, buy B with YML). This keeps the market at **44 pools** instead
of the 946 a full mesh would need, and the app performs the two hops as one action.

The rate below is indicative — units of the currency per 1 YML (YML stands in
for 1 USD on the devnet). It seeds both the pool ratio and the oracle price;
live rates come from the oracle feeder in production.

| Code | Currency | Denom | Symbol | Per YML |
|------|----------|-------|--------|---------|
| YML | Yamale (native) | `uyml` | Y | 1 |
| XOF | West African CFA franc | `uxof` | CFA | 600 |
| XAF | Central African CFA franc | `uxaf` | FCFA | 600 |
| NGN | Nigerian Naira | `ungn` | ₦ | 1550 |
| KES | Kenyan Shilling | `ukes` | KSh | 129 |
| ZAR | South African Rand | `uzar` | R | 18 |
| GHS | Ghanaian Cedi | `ughs` | ₵ | 15 |
| EGP | Egyptian Pound | `uegp` | E£ | 50 |
| MAD | Moroccan Dirham | `umad` | DH | 10 |
| DZD | Algerian Dinar | `udzd` | DA | 135 |
| TND | Tunisian Dinar | `utnd` | DT | 3 |
| ETB | Ethiopian Birr | `uetb` | Br | 120 |
| UGX | Ugandan Shilling | `uugx` | USh | 3700 |
| TZS | Tanzanian Shilling | `utzs` | TSh | 2600 |
| RWF | Rwandan Franc | `urwf` | FRw | 1350 |
| ZMW | Zambian Kwacha | `uzmw` | ZK | 26 |
| MZN | Mozambican Metical | `umzn` | MT | 64 |
| AOA | Angolan Kwanza | `uaoa` | Kz | 910 |
| BWP | Botswana Pula | `ubwp` | P | 13 |
| NAD | Namibian Dollar | `unad` | N$ | 18 |
| MUR | Mauritian Rupee | `umur` | ₨ | 46 |
| GMD | Gambian Dalasi | `ugmd` | D | 70 |
| GNF | Guinean Franc | `ugnf` | FG | 8600 |
| LRD | Liberian Dollar | `ulrd` | L$ | 193 |
| SLE | Sierra Leonean Leone | `usle` | Le | 22 |
| MWK | Malawian Kwacha | `umwk` | MK | 1730 |
| MGA | Malagasy Ariary | `umga` | Ar | 4500 |
| CDF | Congolese Franc | `ucdf` | FC | 2800 |
| SDG | Sudanese Pound | `usdg` | £SD | 600 |
| SSP | South Sudanese Pound | `ussp` | SSP | 4500 |
| LYD | Libyan Dinar | `ulyd` | LD | 5 |
| SOS | Somali Shilling | `usos` | Sh | 571 |
| DJF | Djiboutian Franc | `udjf` | Fdj | 178 |
| ERN | Eritrean Nakfa | `uern` | Nfk | 15 |
| BIF | Burundian Franc | `ubif` | FBu | 2900 |
| SCR | Seychellois Rupee | `uscr` | SR | 13 |
| CVE | Cape Verdean Escudo | `ucve` | $ | 101 |
| STN | São Tomé and Príncipe Dobra | `ustn` | Db | 22 |
| KMF | Comorian Franc | `ukmf` | CF | 450 |
| LSL | Lesotho Loti | `ulsl` | L | 18 |
| SZL | Eswatini Lilangeni | `uszl` | E | 18 |
| MRU | Mauritanian Ouguiya | `umru` | UM | 40 |
| ZWG | Zimbabwe Gold | `uzwg` | ZiG | 13 |
| USDC | USD Coin | `uusdc` | $ | 1 |
| EURC | Euro Coin | `ueurc` | € | 0.92 |

