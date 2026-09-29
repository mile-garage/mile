// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Stroke icons (24×24, currentColor).

const paths: Record<string, string> = {
  calendar: 'M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z',
  // Vehicles, seen from the side and facing right; "vehicle" is the steering wheel for the other kinds.
  car: 'M5 17H3v-4.5l2.5-2 3-3.5h7l3.5 3.5h1.5a2 2 0 0 1 2 2V17h-3M9 17h6M5.5 10.5H19M12 7v3.5M5 17a2 2 0 1 0 4 0 2 2 0 1 0 -4 0M15 17a2 2 0 1 0 4 0 2 2 0 1 0 -4 0',
  van: 'M5 17H2.5V6.5a1 1 0 0 1 1-1H15l4 5h1.5a1 1 0 0 1 1 1V17H19M9 17h6M15 5.5v5h4M2.5 10.5H15M5 17a2 2 0 1 0 4 0 2 2 0 1 0 -4 0M15 17a2 2 0 1 0 4 0 2 2 0 1 0 -4 0',
  truck: 'M4.5 17H2V5h12v12h1.5M8.5 17H14M14 9h4l3 4v4h-1.5M14 13h7M4.5 17a2 2 0 1 0 4 0 2 2 0 1 0 -4 0M15.5 17a2 2 0 1 0 4 0 2 2 0 1 0 -4 0',
  motorcycle: 'M1.5 16a3.5 3.5 0 1 0 7 0 3.5 3.5 0 1 0 -7 0M15.5 16a3.5 3.5 0 1 0 7 0 3.5 3.5 0 1 0 -7 0M4 16a1 1 0 1 0 2 0 1 1 0 1 0 -2 0M18 16a1 1 0 1 0 2 0 1 1 0 1 0 -2 0M9.5 12h3a1 1 0 0 1 1 1v2a1 1 0 0 1-1 1h-3a1 1 0 0 1-1-1v-2a1 1 0 0 1 1-1zM2.5 10.5h6l1.5-2h4.5l2.7 2M14 5.5h3M15.5 5.5 19 16M5 16l3.5-2',
  moped: 'M3 17.5a2.5 2.5 0 1 0 5 0 2.5 2.5 0 1 0 -5 0M16 17.5a2.5 2.5 0 1 0 5 0 2.5 2.5 0 1 0 -5 0M4 9h6.5M4.5 9C2.5 10 2 12.5 2.5 15h11l2-9M14 5.5h3.5M16.5 5.5 18.5 17.5',
  vehicle: 'M3 12a9 9 0 1 0 18 0 9 9 0 1 0 -18 0M10 12a2 2 0 1 0 4 0 2 2 0 1 0 -4 0M3.5 10.5 10 12M14 12l6.5-1.5M12 14v7',
  settings:
    'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z',
  plus: 'M12 5v14M5 12h14',
  back: 'M15 18l-6-6 6-6',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z',
  trash: 'M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6',
  file: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6',
  paperclip: 'M21.4 11.1l-9.2 9.2a6 6 0 0 1-8.5-8.5l9.2-9.2a4 4 0 0 1 5.7 5.7l-9.2 9.2a2 2 0 0 1-2.8-2.8l8.5-8.5',
  camera: 'M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2zM12 17a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  fuel: 'M3 22V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v18M3 22h12M3 10h12M15 8h2a2 2 0 0 1 2 2v7a2 2 0 0 0 4 0V9l-4-4',
  check: 'M20 6L9 17l-5-5',
  pause: 'M6 4h4v16H6zM14 4h4v16h-4z',
  play: 'M5 3l14 9-14 9z',
  gauge: 'M12 14l4-4M3.3 19a10 10 0 1 1 17.4 0',
  bell: 'M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0',
  logout: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9',
  star: 'M12 2l3.1 6.3 6.9 1-5 4.9 1.2 6.8L12 17.8 5.8 21l1.2-6.8-5-4.9 6.9-1z',
  download: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3',
  upload: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12',
  share: 'M16 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM8.6 13.5l6.8 4M15.4 6.5l-6.8 4',
  swap: 'M21 12a9 9 0 0 1-15.5 6.2L3 16M3 12a9 9 0 0 1 15.5-6.2L21 8M21 3v5h-5M3 21v-5h5',
  rotate: 'M7 20V4M3 8l4-4 4 4M17 4v16M21 16l-4 4-4-4',
}

export function Icon({ name, size = 20 }: { name: keyof typeof paths | string; size?: number }) {
  return (
    <svg
      className="icon"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={paths[name]} />
    </svg>
  )
}
