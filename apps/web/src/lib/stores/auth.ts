import { useEffect } from 'react';
import { useSyncExternalStore } from 'react';
import { useNavigate } from 'react-router-dom';

import api from '../api/client';
import {
  completeMfaLogin,
  completeSsoLogin,
  getMe,
  login as apiLogin,
  refreshToken,
  startSsoLogin as apiStartSsoLogin,
  type LoginResponse,
  type MfaRequiredResponse,
  type TokenResponse,
  type UserProfile,
} from '../api/auth';
import { applyUserLocalePreference } from '../i18n/store';

const ACCESS_TOKEN_KEY = 'of_access_token';
const REFRESH_TOKEN_KEY = 'of_refresh_token';
const PENDING_MFA_KEY = 'of_pending_mfa';

type AuthFlowResult = { status: 'authenticated' } | MfaRequiredResponse;

interface AuthSnapshot {
  token: string | null;
  user: UserProfile | null;
  loading: boolean;
  pendingChallenge: MfaRequiredResponse | null;
}

const initialSnapshot: AuthSnapshot = {
  token: null,
  user: null,
  loading: false,
  pendingChallenge: null,
};

let snapshot: AuthSnapshot = initialSnapshot;
const listeners = new Set<() => void>();
let restorePromise: Promise<void> | null = null;

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return snapshot;
}

function setSnapshot(next: Partial<AuthSnapshot>) {
  snapshot = { ...snapshot, ...next };
  listeners.forEach((l) => l());
}

function persistTokens(access: string, refresh: string) {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(ACCESS_TOKEN_KEY, access);
  localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
}

function clearTokens() {
  if (typeof localStorage === 'undefined') return;
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

function persistChallenge(challenge: MfaRequiredResponse) {
  setSnapshot({ pendingChallenge: challenge });
  if (typeof sessionStorage !== 'undefined') {
    sessionStorage.setItem(PENDING_MFA_KEY, JSON.stringify(challenge));
  }
}

function clearPendingChallenge() {
  setSnapshot({ pendingChallenge: null });
  if (typeof sessionStorage !== 'undefined') {
    sessionStorage.removeItem(PENDING_MFA_KEY);
  }
}

function hydratePendingChallenge() {
  if (typeof sessionStorage === 'undefined') return;
  const raw = sessionStorage.getItem(PENDING_MFA_KEY);
  if (!raw) return;
  try {
    const challenge = JSON.parse(raw) as MfaRequiredResponse;
    setSnapshot({ pendingChallenge: challenge });
  } catch {
    clearPendingChallenge();
  }
}

function setSession(resp: TokenResponse) {
  api.setToken(resp.access_token);
  setSnapshot({ token: resp.access_token });
  persistTokens(resp.access_token, resp.refresh_token);
}

function updateCurrentUserProfile(profile: UserProfile) {
  setSnapshot({ user: profile });
  applyUserLocalePreference(profile.attributes);
}

async function finalizeSession(resp: TokenResponse) {
  setSession(resp);
  updateCurrentUserProfile(await getMe());
  clearPendingChallenge();
}

async function handleLoginResponse(resp: LoginResponse): Promise<AuthFlowResult> {
  if (resp.status === 'mfa_required') {
    persistChallenge(resp);
    return resp;
  }
  await finalizeSession(resp);
  return { status: 'authenticated' };
}

async function login(email: string, password: string): Promise<AuthFlowResult> {
  setSnapshot({ loading: true });
  try {
    const resp = await apiLogin({ email, password });
    return handleLoginResponse(resp);
  } finally {
    setSnapshot({ loading: false });
  }
}

async function completeMfa(code: string): Promise<{ status: 'authenticated' }> {
  const challenge = snapshot.pendingChallenge;
  if (!challenge) {
    throw new Error('MFA challenge missing or expired');
  }
  setSnapshot({ loading: true });
  try {
    const resp = await completeMfaLogin({ challenge_token: challenge.challenge_token, code });
    await finalizeSession(resp);
    return { status: 'authenticated' };
  } finally {
    setSnapshot({ loading: false });
  }
}

async function startSsoLogin(slug: string) {
  setSnapshot({ loading: true });
  try {
    const resp = await apiStartSsoLogin(slug);
    if (typeof window !== 'undefined') {
      window.location.assign(resp.authorization_url);
    }
  } finally {
    setSnapshot({ loading: false });
  }
}

async function handleSsoCallback(payload: {
  code?: string;
  state?: string;
  saml_response?: string;
  relay_state?: string;
}): Promise<AuthFlowResult> {
  setSnapshot({ loading: true });
  try {
    const resp = await completeSsoLogin(payload);
    return handleLoginResponse(resp);
  } finally {
    setSnapshot({ loading: false });
  }
}

function logout() {
  api.setToken(null);
  setSnapshot({ token: null, user: null });
  clearTokens();
  clearPendingChallenge();
}

async function restore() {
  if (restorePromise) return restorePromise;

  restorePromise = (async () => {
    setSnapshot({ loading: true });
    try {
      hydratePendingChallenge();

      if (typeof localStorage === 'undefined') return;

      const savedAccess = localStorage.getItem(ACCESS_TOKEN_KEY);
      const savedRefresh = localStorage.getItem(REFRESH_TOKEN_KEY);

      if (savedAccess) {
        api.setToken(savedAccess);
        setSnapshot({ token: savedAccess });
        try {
          updateCurrentUserProfile(await getMe());
          return;
        } catch {
          api.setToken(null);
          setSnapshot({ token: null });
        }
      }

      if (savedRefresh) {
        try {
          const refreshed = await refreshToken(savedRefresh);
          await finalizeSession(refreshed);
        } catch {
          logout();
        }
      } else {
        // Fallback: dev-auth shim does not require a token. If getMe() succeeds
        // anonymously we still treat the user as authenticated for local dev.
        try {
          updateCurrentUserProfile(await getMe());
        } catch {
          // Stay unauthenticated; the page can prompt the user to sign in.
        }
      }
    } finally {
      setSnapshot({ loading: false });
      restorePromise = null;
    }
  })();

  return restorePromise;
}

export const auth = {
  subscribe,
  getSnapshot,
  restore,
  login,
  completeMfa,
  startSsoLogin,
  handleSsoCallback,
  logout,
  clearPendingChallenge,
  updateCurrentUserProfile,
};

export function useAuth() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

export function useCurrentUser() {
  return useAuth().user;
}

export function useIsAuthenticated() {
  const { token, user } = useAuth();
  return Boolean(token) || Boolean(user);
}

export function usePendingMfaChallenge() {
  return useAuth().pendingChallenge;
}

export function useRequireAuth(redirectTo: string = '/auth/login') {
  const { loading, token, user } = useAuth();
  const navigate = useNavigate();
  const authenticated = Boolean(token) || Boolean(user);

  useEffect(() => {
    if (!loading && !authenticated) {
      navigate(redirectTo, { replace: true });
    }
  }, [loading, authenticated, navigate, redirectTo]);

  return { loading, authenticated };
}
