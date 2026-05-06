import { useSyncExternalStore } from 'react';

import { issueNotificationSocketTicket, type NotificationSocketEvent } from '@/lib/api/notifications';

let connected = false;
let socket: WebSocket | null = null;
let requestSerial = 0;
const listeners = new Set<() => void>();

function notify() {
  listeners.forEach((l) => l());
}

function setConnected(next: boolean) {
  if (connected === next) return;
  connected = next;
  notify();
}

function notificationSocketUrl(ticket: string) {
  const env = (import.meta as { env?: Record<string, string> }).env ?? {};
  const configured = env.PUBLIC_NOTIFICATION_WS_URL;
  if (configured) {
    const url = new URL(configured);
    url.searchParams.set('ticket', ticket);
    return url.toString();
  }
  if (typeof window === 'undefined') return '';
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const url = new URL(`${protocol}://${window.location.hostname}:50114/api/v1/notifications/ws`);
  url.searchParams.set('ticket', ticket);
  return url.toString();
}

async function connect(token: string, onMessage: (event: NotificationSocketEvent) => void) {
  if (!token || typeof window === 'undefined') return;
  const serial = ++requestSerial;

  let ticket = '';
  try {
    const response = await issueNotificationSocketTicket();
    ticket = response.ticket;
  } catch {
    setConnected(false);
    return;
  }
  if (serial !== requestSerial) return;

  const nextUrl = notificationSocketUrl(ticket);
  if (!nextUrl) return;
  if (socket && socket.url === nextUrl && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) return;

  disconnect();
  socket = new WebSocket(nextUrl);
  socket.onopen = () => setConnected(true);
  socket.onclose = () => { setConnected(false); socket = null; };
  socket.onerror = () => setConnected(false);
  socket.onmessage = (message) => {
    try {
      onMessage(JSON.parse(String(message.data)) as NotificationSocketEvent);
    } catch {
      /* ignore malformed frames */
    }
  };
}

function disconnect() {
  requestSerial += 1;
  if (socket) {
    socket.close();
    socket = null;
  }
  setConnected(false);
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return connected;
}

export const notificationWebsocket = { connect, disconnect };

export function useNotificationConnected(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
