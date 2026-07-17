import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DocsView } from '../../../src/components/DocsView';
import type { DocIndex } from 'go-ui';

// A minimal DocIndex the stubbed fetch returns for DocsApp's doc.json request.
const DOC_INDEX: DocIndex = {
  module: 'github.com/malcolmston/liveview',
  packages: [
    {
      importPath: 'github.com/malcolmston/liveview',
      name: 'liveview',
      synopsis: 'Package liveview is a standard-library-only reactive server-rendered UI framework.',
      doc: 'Package liveview is a standard-library-only reactive server-rendered UI framework.',
      consts: [],
      vars: [],
      types: [
        {
          name: 'Socket',
          signature: 'type Socket struct{}',
          doc: 'Socket holds the server-side assigns for a connected view.',
          consts: [],
          vars: [],
          funcs: [],
          methods: [],
        },
      ],
      funcs: [{ name: 'NewSession', signature: 'func NewSession(view View) *Session', doc: 'NewSession creates a session bound to view.' }],
    },
  ],
};

describe('DocsView', () => {
  beforeEach(() => {
    // DocsApp fetches doc.json; return the small index.
    global.fetch = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('doc.json')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(DOC_INDEX) } as Response);
      }
      return new Promise<Response>(() => {});
    }) as unknown as typeof fetch;
  });

  it('renders the inline React API reference from the fetched doc.json', async () => {
    const { container } = render(<DocsView />);
    expect(container.querySelector('#view-docs')).not.toBeNull();
    expect(
      screen.getByRole('heading', { level: 2, name: /API documentation/ }),
    ).toBeInTheDocument();

    // DocsApp fetches asynchronously, then renders the package view + symbols.
    expect(await screen.findByRole('heading', { name: /package liveview/ })).toBeInTheDocument();
    expect(container.querySelector('#sym-NewSession'), 'func NewSession symbol card').not.toBeNull();
    expect(container.querySelector('#sym-Socket'), 'type Socket symbol card').not.toBeNull();

    // The secondary link to the raw generated static HTML remains.
    expect(screen.getByRole('link', { name: /Open the raw generated HTML/ })).toHaveAttribute('href', './api/');
  });
});
