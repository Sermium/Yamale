/**
 * Whether the account already holds money: any currency but the network's own
 * token. Such an account is left alone. Since accounts are shared across the
 * Yamale apps (2026-11-26), an account signing in here may be one somebody uses
 * elsewhere with real figures, and filling its other currencies would bury
 * them under test money it never asked for.
 */
export function holdsMoney(held: Map<string, string>): boolean {
  for (const [denom, amount] of held) if (denom !== 'uyml' && BigInt(amount) > 0n) return true;
  return false;
}
