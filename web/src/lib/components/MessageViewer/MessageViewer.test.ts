import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import MessageViewer from './MessageViewer.svelte';

const request =
  'POST /v2/login?next=%2Fhome HTTP/1.1\r\n' +
  'Host: api.acme.test\r\n' +
  'Content-Type: application/x-www-form-urlencoded\r\n' +
  '\r\n' +
  'user=admin&pass=s%26cret';

const response =
  'HTTP/1.1 401 Unauthorized\r\n' +
  'Content-Type: application/json\r\n' +
  '\r\n' +
  '{"error":"invalid_credentials","retry":30}';

describe('MessageViewer', () => {
  it('starts on Raw and shows the message verbatim', () => {
    render(MessageViewer, { props: { raw: request } });
    expect(screen.getByRole('tab', { name: 'Raw' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('region', { name: 'Message' })).toBeVisible();
    expect(document.querySelector('pre')?.textContent).toBe(request);
  });

  it('renders headers and params in Table mode', async () => {
    const user = userEvent.setup();
    render(MessageViewer, { props: { raw: request } });
    await user.click(screen.getByRole('tab', { name: 'Table' }));

    expect(screen.getByRole('rowheader', { name: 'Host' })).toBeVisible();
    expect(screen.getByRole('cell', { name: 'api.acme.test' })).toBeVisible();
    expect(screen.getByRole('cell', { name: 'next' })).toBeVisible();
    expect(screen.getByRole('cell', { name: 's&cret' })).toBeVisible();
  });

  it('renders duplicate headers without crashing', async () => {
    const user = userEvent.setup();
    const smuggling =
      'HTTP/1.1 200 OK\r\nSet-Cookie: a=1\r\nSet-Cookie: a=1\r\nContent-Length: 5\r\n\r\nhello';
    // The viewer renders every tab pane, so a duplicate-key crash would hit the Raw tab too.
    render(MessageViewer, { props: { raw: smuggling } });
    expect(screen.getByRole('tab', { name: 'Raw' })).toBeVisible();

    await user.click(screen.getByRole('tab', { name: 'Table' }));
    expect(screen.getAllByRole('rowheader', { name: 'Set-Cookie' })).toHaveLength(2);
  });

  it('samples form params of a huge urlencoded body', async () => {
    const user = userEvent.setup();
    const form = 'HTTP/1.1 200 OK\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\n';
    render(MessageViewer, { props: { raw: form + 'a=1&'.repeat(1_250_000) } });

    await user.click(screen.getByRole('tab', { name: 'Table' }));
    // 500 param rows + header rows; 1.25M rows would break the 5 MB budget.
    expect(screen.getAllByRole('row').length).toBeLessThan(600);
    expect(screen.getByText(/more params not shown/, { selector: 'p' })).toBeVisible();
  });

  it('pretty-prints JSON bodies', async () => {
    const user = userEvent.setup();
    render(MessageViewer, { props: { raw: response } });
    await user.click(screen.getByRole('tab', { name: 'Pretty' }));

    expect(screen.getByText(/"error": "invalid_credentials"/)).toBeVisible();
  });

  it('renders a hex dump of the body', async () => {
    const user = userEvent.setup();
    render(MessageViewer, { props: { raw: response } });
    await user.click(screen.getByRole('tab', { name: 'Hex' }));

    expect(screen.getByText(/^00000000\s+7b 22 65 72 72 6f 72/)).toBeVisible();
  });

  it('chunks a 5 MB body and grows on demand', async () => {
    const user = userEvent.setup();
    const big = 'HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n' + 'x'.repeat(5_000_000);
    render(MessageViewer, { props: { raw: big } });

    const firstPre = () => document.querySelector('pre')!;
    expect(firstPre().textContent!.length).toBeLessThan(big.length);
    expect(screen.getByText(/Showing 128 KB of 4883 KB/)).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Show more' }));
    expect(firstPre().textContent!.length).toBeGreaterThan(128 * 1024);
  });

  it('searches with Ctrl+F, counts matches and jumps', async () => {
    const user = userEvent.setup();
    render(MessageViewer, { props: { raw: request } });

    await user.click(screen.getByRole('button', { name: 'Search message (Ctrl+F)' }));
    const input = screen.getByLabelText('Search message');
    await user.type(input, 'admin');
    expect(screen.getByText('1/1')).toBeVisible();
    expect(document.querySelector('mark')?.textContent).toBe('admin');

    await user.type(input, 'zzz');
    expect(screen.getByText('No matches')).toBeVisible();
  });

  it('opens search from the keyboard', async () => {
    render(MessageViewer, { props: { raw: request } });
    await fireEvent.keyDown(screen.getByRole('region', { name: 'Message' }), {
      key: 'f',
      ctrlKey: true,
    });

    const input = await screen.findByLabelText('Search message');
    await waitFor(() => expect(input).toHaveFocus());
  });

  it('offers a sandboxed preview only when a URL is given', async () => {
    const user = userEvent.setup();
    const { unmount } = render(MessageViewer, {
      props: { raw: response, title: 'Response', previewUrl: '/preview/42' },
    });
    await user.click(screen.getByRole('tab', { name: 'Preview' }));
    expect(screen.getByTitle('Response preview')).toHaveAttribute('sandbox', '');
    unmount();

    render(MessageViewer, { props: { raw: response } });
    expect(screen.queryByRole('tab', { name: 'Preview' })).toBeNull();
  });
});
