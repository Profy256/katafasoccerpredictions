import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { ImageResponse } from 'next/og';

export const alt = 'Katafa — football predictions with a public accuracy record';
export const size = { width: 1200, height: 630 };
export const contentType = 'image/png';

// Satori has no filesystem access, so the mark is inlined as a data URL. Read
// at module scope: this runs once at build time, not per render.
const mark = `data:image/png;base64,${readFileSync(
  join(process.cwd(), 'public/brand/katafa-mark-512.png'),
).toString('base64')}`;

export default function OpengraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          padding: '0 96px',
          background: '#0a0e15',
          color: '#e8eef6',
          fontFamily: 'sans-serif',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 36 }}>
          {/* Satori renders plain <img>; next/image has no meaning here. */}
          <img src={mark} width={168} height={168} alt="" />
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            <div style={{ fontSize: 92, fontWeight: 700, letterSpacing: -3 }}>
              Katafa
            </div>
            <div style={{ fontSize: 34, color: '#93a3b8', marginTop: 4 }}>
              Football predictions with receipts
            </div>
          </div>
        </div>

        <div
          style={{
            width: 120,
            height: 6,
            background: '#16c834',
            margin: '56px 0 32px',
          }}
        />

        <div style={{ fontSize: 30, color: '#93a3b8', lineHeight: 1.4 }}>
          Every pick is graded against the real result and kept on the record —
          including the ones that lost.
        </div>
      </div>
    ),
    size,
  );
}
