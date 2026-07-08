// ── Authentication Module ────────────────────────────────────────────

import { state } from './state.js';
import { apiGet } from './api.js';

const API_BASE = '/api/gantt';

export async function checkAuth() {
    // 1. Check if token is in URL (e.g. redirected or sent from bot)
    const params = new URLSearchParams(window.location.search);
    const urlToken = params.get('token');
    if (urlToken) {
        localStorage.setItem('tg_sys_token', urlToken);
        // Clean URL params
        window.history.replaceState({}, document.title, window.location.pathname);
    }

    // 2. Check localStorage token
    let token = localStorage.getItem('tg_sys_token');
    
    // 3. Check Telegram WebApp initData if inside Telegram
    if (!token && window.Telegram && window.Telegram.WebApp && window.Telegram.WebApp.initData) {
        try {
            const resp = await fetch(`${API_BASE}/auth/webapp`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ initData: window.Telegram.WebApp.initData }),
            });
            if (resp.ok) {
                const data = await resp.json();
                if (data.token) {
                    localStorage.setItem('tg_sys_token', data.token);
                    token = data.token;
                }
            }
        } catch (e) {
            console.error('Failed to authenticate via Telegram WebApp:', e);
        }
    }

    // 4. Fallback: check query params for hash from Telegram Widget (legacy redirect)
    if (!token && params.get('hash')) {
        // Let the widget callback do its work. 
        // We will reload to let the browser send credentials if cookie is set.
        // Wait briefly, or try to request profile since the server might have set a cookie.
    }

    if (token || document.cookie.includes('tg_sys_auth')) {
        try {
            // Load user profile and role
            const profile = await apiGet('/profile');
            state.set('userProfile', profile);
            showApp();
            return;
        } catch (e) {
            console.error('Profile fetch failed:', e);
        }
    }

    // If we reach here, we are not authenticated
    showAuth();
}

export function logout() {
    localStorage.removeItem('tg_sys_token');
    // Clear cookies if possible
    document.cookie = 'tg_sys_auth=; Path=/; Max-Age=-1;';
    state.set('userProfile', null);
    showAuth();
}

function showAuth() {
    document.getElementById('loading-overlay').classList.add('hidden');
    document.getElementById('auth-overlay').classList.remove('hidden');
    document.getElementById('app').classList.add('hidden');
    document.getElementById('denied-overlay').classList.add('hidden');
}

function showApp() {
    document.getElementById('loading-overlay').classList.add('hidden');
    document.getElementById('auth-overlay').classList.add('hidden');
    document.getElementById('denied-overlay').classList.add('hidden');
    document.getElementById('app').classList.remove('hidden');
}
