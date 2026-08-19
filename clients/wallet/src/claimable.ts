// GENERATED from scripts/currencies/african-currencies.json -- do not hand-edit.

export interface ClaimGroup {
  id: string;
  title: string;
  denoms: readonly string[];
}

/** Every denom the faucet is stocked with, in one flat list. */
export const CLAIMABLE = [
  "uyml",
  "uxof",
  "uxaf",
  "ungn",
  "ukes",
  "uzar",
  "ughs",
  "uegp",
  "umad",
  "udzd",
  "utnd",
  "uetb",
  "uugx",
  "utzs",
  "urwf",
  "uzmw",
  "umzn",
  "uaoa",
  "ubwp",
  "unad",
  "umur",
  "ugmd",
  "ugnf",
  "ulrd",
  "usle",
  "umwk",
  "umga",
  "ucdf",
  "usdg",
  "ussp",
  "ulyd",
  "usos",
  "udjf",
  "uern",
  "ubif",
  "uscr",
  "ucve",
  "ustn",
  "ukmf",
  "ulsl",
  "uszl",
  "umru",
  "uzwg",
  "uusdc",
  "ueurc",
] as const;

export type Denom = (typeof CLAIMABLE)[number];

/** The same set, grouped for the page: reserves first, then by region. */
export const CLAIM_GROUPS: ClaimGroup[] = [
  { id: 'reserve', title: 'Reserve and stablecoins', denoms: ["uyml", "uusdc", "ueurc"] },
  { id: "west", title: "West Africa", denoms: ["uxof", "ungn", "ughs", "ugmd", "ugnf", "ulrd", "usle", "ucve", "umru"] },
  { id: "central", title: "Central Africa", denoms: ["uxaf", "uaoa", "ucdf", "ustn"] },
  { id: "east", title: "East Africa", denoms: ["ukes", "uetb", "uugx", "utzs", "urwf", "usdg", "ussp", "usos", "udjf", "uern", "ubif"] },
  { id: "north", title: "North Africa", denoms: ["uegp", "umad", "udzd", "utnd", "ulyd"] },
  { id: "southern", title: "Southern Africa", denoms: ["uzar", "uzmw", "umzn", "ubwp", "unad", "umwk", "ulsl", "uszl", "uzwg"] },
  { id: "indian", title: "Indian Ocean", denoms: ["umur", "umga", "uscr", "ukmf"] },
];
