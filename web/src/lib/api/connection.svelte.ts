// Live connection state for the status bar: WebSocket lifecycle plus the server's
// build facts (T-007, docs/06-frontend-conventions.md §4).
import { isUnauthenticatedClose, type Api } from './client';
import { PingDocument, ServerInfoDocument, type ServerInfoQuery } from './generated/graphql';

export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'unauthorized';
export type ServerInfo = ServerInfoQuery['serverInfo'];

export class ConnectionState {
  status = $state<ConnectionStatus>('connecting');
  server = $state.raw<ServerInfo | null>(null);
  /** Server time of the last ping, as sent (RFC 3339). */
  lastPing = $state<string | null>(null);

  /**
   * Follows the WebSocket of `api` and keeps the `ping` subscription open, which
   * also keeps the socket from closing lazily. Returns the cleanup.
   */
  start(api: Api): () => void {
    const offs = [
      api.ws.on('connecting', () => {
        if (this.status !== 'unauthorized') this.status = 'connecting';
      }),
      api.ws.on('connected', () => {
        this.status = 'connected';
        void this.loadServerInfo(api);
      }),
      api.ws.on('closed', (e) => {
        this.status = isUnauthenticatedClose(e) ? 'unauthorized' : 'disconnected';
      }),
    ];

    const ping = api.client.subscription(PingDocument, {}).subscribe((result) => {
      if (result.data) this.lastPing = result.data.ping.at;
    });

    return () => {
      ping.unsubscribe();
      for (const off of offs) off();
    };
  }

  private async loadServerInfo(api: Api): Promise<void> {
    const result = await api.client
      .query(ServerInfoDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (result.data) this.server = result.data.serverInfo;
    else if (result.error?.response?.status === 401) this.status = 'unauthorized';
  }
}

export const connection = new ConnectionState();
