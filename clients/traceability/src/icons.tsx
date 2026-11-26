/**
 * The app's icons: one set, drawn on a 24-unit grid with a 1.75 stroke, round
 * caps and joins, in currentColor. Nothing else in the app draws an icon.
 */

const PATHS = {
  verify: ['M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14Z', 'm20 20-4-4', 'm8.5 11 1.8 1.8 3.4-3.6'],
  market: ['M5 8h14l-1.2 12H6.2L5 8Z', 'M9 8a3 3 0 0 1 6 0'],
  jewelers: ['M3.5 9 5 4h14l1.5 5', 'M3.5 9h17', 'M5 9v11h14V9', 'M10 20v-5h4v5'],
  gem: ['M6.5 4h11l3.5 5-9 11L3 9l3.5-5Z', 'M3 9h18', 'M10 4 8 9l4 11 4-11-2-5'],
  atelier: ['M14.5 6.5a4 4 0 0 0-5.3 5.3L3.5 17.5l3 3 5.7-5.7a4 4 0 0 0 5.3-5.3l-2.4 2.4-2.6-.4-.4-2.6 2.4-2.4Z'],
  account: ['M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8Z', 'M4.5 20.5a7.5 7.5 0 0 1 15 0'],
  plus: ['M12 5v14', 'M5 12h14'],
  catalogue: ['M5 5.5A2.5 2.5 0 0 1 7.5 3H19v15H7.5A2.5 2.5 0 0 0 5 20.5v-15Z', 'M5 20.5A2.5 2.5 0 0 1 7.5 18H19v3H7.5', 'M9 7h6'],
  card: ['M5 5h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z', 'M7 10h5', 'M7 14h3', 'M16 10.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Z'],
  ticket: ['M4 6h16v3.5a2.5 2.5 0 0 0 0 5V18H4v-3.5a2.5 2.5 0 0 0 0-5V6Z', 'M14.5 6v2', 'M14.5 11v2', 'M14.5 16v2'],
  palette: ['M12 3.5a8.5 8.5 0 1 0 0 17c1.1 0 1.8-.8 1.8-1.7 0-.5-.2-.9-.5-1.2-.3-.3-.5-.7-.5-1.2 0-1 .8-1.7 1.8-1.7h2.1a3.8 3.8 0 0 0 3.8-3.8C20.5 6.8 16.7 3.5 12 3.5Z', 'M7.5 12h.01', 'M9.5 8h.01', 'M14 7.5h.01'],
  team: ['M9 4.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Z', 'M2.5 20a6.5 6.5 0 0 1 13 0', 'M16 4.7a3.5 3.5 0 0 1 0 6.6', 'M18.5 14.4a6.5 6.5 0 0 1 3 5.6'],
  chevron: ['m9.5 6 6 6-6 6'],
  lock: ['M6 11h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8a1 1 0 0 1 1-1Z', 'M8 11V7.5a4 4 0 0 1 8 0V11'],
  logout: ['M14.5 4H19v16h-4.5', 'M10 8l-4 4 4 4', 'M6 12h10'],
  shield: ['M12 3 19.5 6v5.5c0 4.6-3.2 8.2-7.5 9.5-4.3-1.3-7.5-4.9-7.5-9.5V6L12 3Z', 'm8.8 12 2.2 2.2 4.2-4.4'],
  qr: ['M4 4h6v6H4V4Z', 'M14 4h6v6h-6V4Z', 'M4 14h6v6H4v-6Z', 'M14 14h2.5v2.5H14V14Z', 'M17.5 17.5H20V20h-2.5v-2.5Z', 'M14 18.5V20', 'M18.5 14H20'],
  inbox: ['M3.5 13.5h5l1.5 2.5h4l1.5-2.5h5', 'M6 5h12l2.5 8.5V19a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1v-5.5L6 5Z'],
  sun: ['M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8Z', 'M12 2.5v2', 'M12 19.5v2', 'm5.3 5.3 1.4 1.4', 'm17.3 17.3 1.4 1.4', 'M2.5 12h2', 'M19.5 12h2', 'm5.3 18.7 1.4-1.4', 'm17.3 6.7 1.4-1.4'],
  moon: ['M19.5 14.5A8 8 0 0 1 9.5 4.5a8 8 0 1 0 10 10Z'],
  key: ['M8.5 11.5a4 4 0 1 0 0 8 4 4 0 0 0 0-8Z', 'm11.4 12.6 8.1-8.1', 'm16.5 7.5 2.5 2.5', 'm14 10 1.8 1.8'],
} as const;

export type IconName = keyof typeof PATHS;

export function Icon({ name, size = 20, className }: { name: IconName; size?: number; className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" width={size} height={size} fill="none" stroke="currentColor"
         strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      {PATHS[name].map((d) => <path key={d} d={d} />)}
    </svg>
  );
}
