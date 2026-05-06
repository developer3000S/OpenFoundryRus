import { useEffect, useMemo, useState } from 'react';

import {
  getNotificationPreferences,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  sendNotification,
  updateNotificationPreferences,
  type NotificationPreference,
  type NotificationSocketEvent,
  type UserNotification,
} from '@/lib/api/notifications';
import { formatDateTime } from '@/lib/i18n/format';
import { createTranslator, useCurrentLocale } from '@/lib/i18n/store';
import { useAuth } from '@/lib/stores/auth';
import { notificationWebsocket, useNotificationConnected } from '@/lib/stores/notificationWebsocket';
import { notifications as toasts } from '@/lib/stores/notifications';

const DEFAULT_PREFS: NotificationPreference = {
  user_id: '',
  in_app_enabled: true,
  email_enabled: false,
  email_address: null,
  slack_webhook_url: null,
  teams_webhook_url: null,
  digest_frequency: 'instant',
  quiet_hours: {},
  updated_at: '',
};

export function NotificationBell() {
  const locale = useCurrentLocale();
  const t = useMemo(() => createTranslator(locale), [locale]);
  const { token, user } = useAuth();
  const isAuthenticated = !!(token && user);
  const connected = useNotificationConnected();

  const [open, setOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<'inbox' | 'preferences'>('inbox');
  const [items, setItems] = useState<UserNotification[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [sendingTest, setSendingTest] = useState(false);
  const [error, setError] = useState('');
  const [preferences, setPreferences] = useState<NotificationPreference>(DEFAULT_PREFS);

  function upsertNotification(n: UserNotification) {
    setItems((prev) => [n, ...prev.filter((it) => it.id !== n.id)].slice(0, 20));
  }

  async function loadInbox() {
    setLoading(true);
    try {
      const response = await listNotifications({ limit: 20 });
      setItems(response.data);
      setUnreadCount(response.unread_count);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('notifications.failedLoad'));
    } finally {
      setLoading(false);
    }
  }

  async function loadPreferences() {
    try {
      setPreferences(await getNotificationPreferences());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('notifications.failedPreferences'));
    }
  }

  function applySocketEvent(event: NotificationSocketEvent) {
    setUnreadCount(event.unread_count);
    if (event.kind === 'snapshot') {
      setItems(event.data ?? []);
      return;
    }
    if (event.notification) {
      upsertNotification(event.notification);
      if (event.kind === 'notification.created') toasts.info(event.notification.title);
    }
    if (event.kind === 'notification.read_all') {
      setItems((prev) => prev.map((it) => ({ ...it, status: 'read', read_at: it.read_at ?? new Date().toISOString() })));
    }
  }

  useEffect(() => {
    if (isAuthenticated && token) {
      void loadInbox();
      void loadPreferences();
      void notificationWebsocket.connect(token, applySocketEvent);
      return () => notificationWebsocket.disconnect();
    }
    notificationWebsocket.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, token]);

  async function markRead(id: string) {
    const response = await markNotificationRead(id);
    setUnreadCount(response.unread_count);
    upsertNotification(response.notification);
  }

  async function markEverythingRead() {
    const response = await markAllNotificationsRead();
    setUnreadCount(response.unread_count);
    setItems((prev) => prev.map((it) => ({ ...it, status: 'read', read_at: it.read_at ?? new Date().toISOString() })));
  }

  async function savePreferences() {
    setSaving(true);
    setError('');
    try {
      const updated = await updateNotificationPreferences({
        in_app_enabled: preferences.in_app_enabled,
        email_enabled: preferences.email_enabled,
        email_address: preferences.email_address,
        slack_webhook_url: preferences.slack_webhook_url,
        teams_webhook_url: preferences.teams_webhook_url,
        digest_frequency: preferences.digest_frequency,
        quiet_hours: preferences.quiet_hours,
      });
      setPreferences(updated);
      toasts.success(t('notifications.preferencesUpdated'));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('notifications.failedUpdate'));
    } finally {
      setSaving(false);
    }
  }

  async function sendTestNotification() {
    setSendingTest(true);
    setError('');
    try {
      const notification = await sendNotification({
        title: t('notifications.testTitle'),
        body: t('notifications.testBody'),
        category: 'test',
        severity: 'info',
        channels: ['in_app'],
      });
      upsertNotification(notification);
      setUnreadCount((c) => c + 1);
      toasts.success(t('notifications.testSent'));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('notifications.testFailed'));
    } finally {
      setSendingTest(false);
    }
  }

  if (!isAuthenticated) return null;

  return (
    <div className="relative">
      <button
        type="button"
        aria-label={t('notifications.ariaOpen')}
        onClick={() => { setOpen((v) => !v); if (!open) void loadInbox(); }}
        className="relative rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm shadow-sm transition-colors hover:bg-slate-50 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800"
      >
        <span>🔔</span>
        {unreadCount > 0 && (
          <span className="absolute -right-1 -top-1 rounded-full bg-rose-500 px-1.5 py-0.5 text-[10px] font-semibold text-white">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-14 z-20 w-[24rem] rounded-2xl border border-slate-200 bg-white p-4 shadow-2xl dark:border-gray-700 dark:bg-gray-900">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-xs uppercase tracking-[0.22em] text-gray-400">{t('notifications.center')}</div>
              <div className="mt-1 text-sm text-gray-500">{t('notifications.subtitle')}</div>
            </div>
            <div className="flex items-center gap-2 text-xs text-gray-500">
              <span className={`h-2.5 w-2.5 rounded-full ${connected ? 'bg-emerald-500' : 'bg-amber-500'}`} />
              {connected ? t('common.online') : t('common.offline')}
            </div>
          </div>

          <div className="mt-4 flex gap-2 rounded-xl bg-slate-100 p-1 dark:bg-gray-800">
            <button
              type="button"
              onClick={() => setActiveTab('inbox')}
              className={`flex-1 rounded-lg px-3 py-2 text-sm font-medium ${activeTab === 'inbox' ? 'bg-white shadow-sm dark:bg-gray-900' : 'text-gray-500'}`}
            >{t('notifications.inbox')}</button>
            <button
              type="button"
              onClick={() => setActiveTab('preferences')}
              className={`flex-1 rounded-lg px-3 py-2 text-sm font-medium ${activeTab === 'preferences' ? 'bg-white shadow-sm dark:bg-gray-900' : 'text-gray-500'}`}
            >{t('notifications.preferences')}</button>
          </div>

          {error && (
            <div className="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:border-rose-900/40 dark:bg-rose-950/40 dark:text-rose-300">{error}</div>
          )}

          {activeTab === 'inbox' ? (
            <div className="mt-4 space-y-3">
              <div className="flex items-center justify-between text-sm text-gray-500">
                <span>{t('notifications.unreadCount', { count: unreadCount })}</span>
                <div className="flex gap-2">
                  <button type="button" onClick={() => void sendTestNotification()} disabled={sendingTest} className="rounded-lg border border-slate-200 px-3 py-1.5 hover:bg-slate-50 disabled:opacity-50 dark:border-gray-700 dark:hover:bg-gray-800">
                    {sendingTest ? t('notifications.sendingTest') : t('notifications.sendTest')}
                  </button>
                  <button type="button" onClick={() => void markEverythingRead()} className="rounded-lg border border-slate-200 px-3 py-1.5 hover:bg-slate-50 dark:border-gray-700 dark:hover:bg-gray-800">
                    {t('notifications.markAllRead')}
                  </button>
                </div>
              </div>

              {loading ? (
                <div className="py-8 text-center text-sm text-gray-500">{t('notifications.loading')}</div>
              ) : items.length === 0 ? (
                <div className="rounded-xl border border-dashed border-slate-300 px-4 py-8 text-center text-sm text-gray-500 dark:border-gray-700">{t('notifications.empty')}</div>
              ) : (
                <div className="max-h-[22rem] space-y-3 overflow-auto pr-1">
                  {items.map((item) => (
                    <div key={item.id} className={`rounded-xl border p-3 ${item.status === 'unread' ? 'border-blue-200 bg-blue-50/70 dark:border-blue-900/40 dark:bg-blue-950/30' : 'border-slate-200 dark:border-gray-700'}`}>
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <div className="flex flex-wrap items-center gap-2">
                            <div className="font-medium">{item.title}</div>
                            <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.16em] text-slate-600 dark:bg-gray-800 dark:text-gray-300">{item.category}</span>
                          </div>
                          <div className="mt-1 text-sm text-gray-600 dark:text-gray-300">{item.body}</div>
                          <div className="mt-2 flex flex-wrap gap-2 text-xs text-gray-500">
                            <span>{formatDateTime(item.created_at, locale)}</span>
                            <span>{Array.isArray(item.channels) ? item.channels.join(', ') : ''}</span>
                          </div>
                        </div>
                        {item.status === 'unread' && (
                          <button type="button" onClick={() => void markRead(item.id)} className="rounded-lg border border-slate-200 px-2.5 py-1 text-xs hover:bg-slate-50 dark:border-gray-700 dark:hover:bg-gray-800">
                            {t('notifications.read')}
                          </button>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ) : (
            <div className="mt-4 space-y-4 text-sm">
              <label className="flex items-center justify-between rounded-xl border border-slate-200 px-3 py-2 dark:border-gray-700">
                <span>{t('notifications.enableInApp')}</span>
                <input type="checkbox" checked={preferences.in_app_enabled} onChange={(e) => setPreferences((p) => ({ ...p, in_app_enabled: e.target.checked }))} />
              </label>
              <label className="flex items-center justify-between rounded-xl border border-slate-200 px-3 py-2 dark:border-gray-700">
                <span>{t('notifications.enableEmail')}</span>
                <input type="checkbox" checked={preferences.email_enabled} onChange={(e) => setPreferences((p) => ({ ...p, email_enabled: e.target.checked }))} />
              </label>
              <input
                value={preferences.email_address ?? ''}
                onChange={(e) => setPreferences((p) => ({ ...p, email_address: e.target.value || null }))}
                placeholder={t('notifications.emailAddress')}
                className="w-full rounded-xl border border-slate-200 px-3 py-2 dark:border-gray-700 dark:bg-gray-800"
              />
              <input
                value={preferences.slack_webhook_url ?? ''}
                onChange={(e) => setPreferences((p) => ({ ...p, slack_webhook_url: e.target.value || null }))}
                placeholder={t('notifications.slackWebhook')}
                className="w-full rounded-xl border border-slate-200 px-3 py-2 dark:border-gray-700 dark:bg-gray-800"
              />
              <input
                value={preferences.teams_webhook_url ?? ''}
                onChange={(e) => setPreferences((p) => ({ ...p, teams_webhook_url: e.target.value || null }))}
                placeholder={t('notifications.teamsWebhook')}
                className="w-full rounded-xl border border-slate-200 px-3 py-2 dark:border-gray-700 dark:bg-gray-800"
              />
              <select
                value={preferences.digest_frequency}
                onChange={(e) => setPreferences((p) => ({ ...p, digest_frequency: e.target.value }))}
                className="w-full rounded-xl border border-slate-200 px-3 py-2 dark:border-gray-700 dark:bg-gray-800"
              >
                <option value="instant">{t('notifications.digest.instant')}</option>
                <option value="hourly">{t('notifications.digest.hourly')}</option>
                <option value="daily">{t('notifications.digest.daily')}</option>
              </select>
              <button type="button" onClick={() => void savePreferences()} disabled={saving} className="w-full rounded-xl bg-slate-900 px-4 py-2 text-white disabled:opacity-50 dark:bg-white dark:text-slate-900">
                {saving ? t('common.saving') : t('notifications.savePreferences')}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
